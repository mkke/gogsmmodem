package gogsmmodem

import (
	"fmt"
	"log"
	"reflect"
	"testing"
	"time"
)

var initReplay = []string{
	"->ATZ\r\n",
	"<-\r\nOK\r\n",
	"->ATE0\r\n",
	"<-ATE0\n",
	"<-\r\nOK\r\n",
	"->AT+CPMS=\"SM\",\"SM\",\"SM\"\r\n",
	"<-\r\n+CPMS: 50,50,50,50,50,50\r\nOK\n\n",
	"->AT+CMGF=1\r\n",
	"<-\r\nOK\r\n",
	"->AT+CNMI=0,1,0,1,1\r\n",
	"<-\r\nOK\r\n",
	"->AT+CSCA?\r\n",
	"<-\r\n+CSCA: \"+447802092035\",145\r\nOK\r\n",
	"->AT+CSCA=\"+447802092035\",145\r\n",
	"<-\r\nOK\r\n",
}

func appendLists(ls ...[]string) []string {
	size := 0
	for _, l := range ls {
		size += len(l)
	}
	ret := make([]string, size)
	off := ret
	for _, l := range ls {
		copy(off, l)
		off = off[len(l):]
	}
	return ret
}

func TestInit(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}
	modem.Close()
}

// assertOOBCommands reads commands from modem.OOB in order, closes modem, and
// checks that OOB then closes without a further packet.
func assertOOBCommands(t *testing.T, modem *Modem, commands []Packet) {
	t.Helper()
	for _, want := range commands {
		select {
		case got, ok := <-modem.OOB:
			if !ok {
				t.Fatalf("OOB closed, expected: %#v", want)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Expected: %#v, got: %#v", want, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("no packet on OOB, expected: %#v", want)
		}
	}
	modem.Close()
	for extra := range modem.OOB {
		t.Errorf("Unexpected extra command: %#v", extra)
	}
}

var oobReplay = []string{
	"<-\r\n+ZUSIMR:2\r\n",
	"<-\r\n+ZPASR: \"No Service\"\r\n",
	"<-\r\n+ZDONR: \"O2-UK\",234,10,\"CS_PS\",\"ROAM_OFF\"\r\n",
	"<-\r\n+ZPASR: \"EDGE\"\r\n",
	"<-\r\n+ZPASR: \"UMTS\"\r\n",
	"<-\r\nDODGY\r\n",
	"<-\r\n+ZZZ: \"A\"\r\n",
}

var oobCommands = []Packet{
	ServiceStatus{"No Service"},
	NetworkStatus{"O2-UK"},
	ServiceStatus{"EDGE"},
	ServiceStatus{"UMTS"},
	UnknownPacket{"DODGY", []any{}},
	UnknownPacket{"+ZZZ", []any{"A"}},
}

func TestOOB(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(oobReplay, initReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}
	assertOOBCommands(t, modem, oobCommands)
}

var receivedReplay = []string{
	"<-\r\n+CMTI: \"SM\",5\r\n",
}

var receivedCommands = []Packet{
	MessageNotification{"SM", 5},
}

func TestIncoming(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, receivedReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}
	assertOOBCommands(t, modem, receivedCommands)
}

// A Close while listen waits to put a packet on a full OOB returns, and OOB
// then closes after the packets it holds.
func TestCloseStopsListen(t *testing.T) {
	// More reports than OOB holds.
	var replay []string
	for i := 0; i < 20; i++ {
		replay = append(replay, "<-\r\n+CMTI: \"SM\",5\r\n")
	}
	modem, err := New(NewMockSerialPort(appendLists(initReplay, replay)), false)
	if err != nil {
		t.Fatal("Expected: no error, got:", err)
	}
	for deadline := time.Now().Add(time.Second); len(modem.OOB) < cap(modem.OOB); {
		if time.Now().After(deadline) {
			t.Fatalf("OOB holds %d packets, want %d", len(modem.OOB), cap(modem.OOB))
		}
		time.Sleep(time.Millisecond)
	}
	closed := make(chan error)
	go func() { closed <- modem.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("Close() = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not return")
	}
	for range modem.OOB {
	}
	if err := modem.Close(); err != nil {
		t.Errorf("second Close() = %v", err)
	}
	if _, err := modem.GetMessage(1); err != ErrClosed {
		t.Errorf("GetMessage after Close: err = %v, want ErrClosed", err)
	}
}

// withCommandTimeout sets commandTimeout to d until the test ends.
func withCommandTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := commandTimeout
	commandTimeout = d
	t.Cleanup(func() { commandTimeout = prev })
}

// A modem that does not answer the reset makes New return ErrTimeout.
func TestInitTimeout(t *testing.T) {
	withCommandTimeout(t, 20*time.Millisecond)
	port := NewMockSerialPort([]string{"->ATZ\r\n"})
	if _, err := New(port, false); err != ErrTimeout {
		t.Errorf("New: err = %v, want ErrTimeout", err)
	}
	port.Close()
}

// A command without an answer returns ErrTimeout and stops listen: OOB
// closes and the next command returns ErrClosed.
func TestCommandTimeout(t *testing.T) {
	withCommandTimeout(t, 20*time.Millisecond)
	modem, err := New(NewMockSerialPort(appendLists(initReplay, []string{"->AT+CMGR=1\r\n"})), false)
	if err != nil {
		t.Fatal("Expected: no error, got:", err)
	}
	if _, err := modem.GetMessage(1); err != ErrTimeout {
		t.Errorf("GetMessage: err = %v, want ErrTimeout", err)
	}
	select {
	case _, ok := <-modem.OOB:
		if ok {
			t.Error("Expected: OOB closed, got a packet")
		}
	case <-time.After(time.Second):
		t.Fatal("OOB not closed after a timeout")
	}
	if _, err := modem.GetMessage(1); err != ErrClosed {
		t.Errorf("GetMessage after the timeout: err = %v, want ErrClosed", err)
	}
	if err := modem.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

// A failed read from the port, as when a USB modem is pulled, stops listen:
// OOB closes and a command returns ErrClosed.
func TestReadErrorClosesOOB(t *testing.T) {
	port := NewMockSerialPort(appendLists(initReplay))
	modem, err := New(port, false)
	if err != nil {
		t.Fatal("Expected: no error, got:", err)
	}
	port.Close()
	select {
	case _, ok := <-modem.OOB:
		if ok {
			t.Error("Expected: OOB closed, got a packet")
		}
	case <-time.After(time.Second):
		t.Fatal("OOB not closed after a read error")
	}
	if _, err := modem.GetMessage(1); err != ErrClosed {
		t.Errorf("GetMessage: err = %v, want ErrClosed", err)
	}
	modem.Close()
}

var messageReplay = []string{
	"->AT+CMGR=1\r\n",
	"<-\r\n+CMGR: \"REC UNREAD\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nHi\r\n\r\nOK\r\n",
}

func TestGetMessage(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, messageReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.GetMessage(1)
	expected := Message{0, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Hi", false}
	if *msg != expected {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}
	modem.Close()
}

var missingMessageReplay = []string{
	"->AT+CMGR=1\r\n",
	"<-\r\nOK\r\n",
}

func TestGetMissingMessage(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, missingMessageReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	_, err = modem.GetMessage(1)
	if fmt.Sprint(err) != "Message not found" {
		t.Errorf("Expected error: %#v, got %#v", err, err)
	}
	modem.Close()
}

var sendMessageReplay = []string{
	"->AT+CMGS=\"441234567890\"\r\n",
	"<-> \r\n",
	"->Body\x00\x1a",
	"<-\r\nOK\r\n",
}

func TestSendMessage(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, sendMessageReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	err = modem.SendMessage("441234567890", "Body@")
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}
	modem.Close()
}

var listMessagesReplay = []string{
	"->AT+CMGL=\"ALL\"\r\n",
	"<-\r\n+CMGL: 0,\"REC UNREAD\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nHi\r\n+CMGL: 1,\"REC READ\",\"+441234567890\",,\"14/02/01,15:07:43+00\"\r\nOla\r\n+CMGL: 2,\"REC UNREAD\",\"+44123456",
	"<-7890\",,\"14/02/01,15:07:43+00\"\r\nJa\r\n\r\nOK\r\n",
}

func TestListMessages(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, listMessagesReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.ListMessages("ALL")
	expected := MessageList{
		Message{0, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Hi", false},
		Message{1, "REC READ", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Ola", false},
		Message{2, "REC UNREAD", "+441234567890", time.Date(2014, 2, 1, 15, 7, 43, 0, time.UTC), "Ja", true},
	}
	if len(*msg) != len(expected) {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}
	for i, m := range *msg {
		if m != expected[i] {
			t.Errorf("Expected: %#v, got %#v", expected, msg)
		}
	}
	modem.Close()
}

var listMessagesEmptyReplay = []string{
	"->AT+CMGL=\"ALL\"\r\n",
	"<-\r\nOK\r\n",
}

func TestListMessagesEmpty(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, listMessagesEmptyReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, err := modem.ListMessages("ALL")
	log.Println("ERROR", err)
	expected := MessageList{}
	if len(*msg) != len(expected) {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}
	modem.Close()
}

var storageAreasReplay = []string{
	"->AT+CPMS=?\r\n",
	"<-\r\n+CPMS: (\"ME\",\"MT\",\"SM\",\"SR\"),(\"ME\",\"MT\",\"SM\",\"SR\"),(\"ME\",\"MT\",\"SM\",\"SR\")\r\n\r\nOK\r\n",
}

func TestSupportedStorageAreas(t *testing.T) {
	modem, err := New(NewMockSerialPort(appendLists(initReplay, storageAreasReplay)), true)
	if err != nil {
		t.Error("Expected: no error, got:", err)
	}

	msg, _ := modem.SupportedStorageAreas()
	expected := StorageAreas{
		[]string{"ME", "MT", "SM", "SR"},
		[]string{"ME", "MT", "SM", "SR"},
		[]string{"ME", "MT", "SM", "SR"},
	}
	got, want := fmt.Sprint(*msg), fmt.Sprint(expected)
	if got != want {
		t.Errorf("Expected: %#v, got %#v", expected, msg)
	}
	modem.Close()
}

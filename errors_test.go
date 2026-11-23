package gogsmmodem

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A modem without a SIM refuses the storage selection of New with CME error
// 10, and New returns that as a *ModemError naming the command and the code.
// Once the caller closes the port, no goroutine of New is left.
func TestInitWithoutSIM(t *testing.T) {
	before := runtime.NumGoroutine()
	replay := []string{
		"->ATZ\r\n",
		"<-\r\nOK\r\n",
		"->ATE0\r\n",
		"<-\r\nOK\r\n",
		"->AT+CPMS=\"SM\",\"SM\",\"SM\"\r\n",
		"<-\r\n+CME ERROR: 10\r\n",
		"->AT+CPMS=\"SM\",\"SM\"\r\n",
		"<-\r\n+CME ERROR: 10\r\n",
	}
	port := NewMockSerialPort(replay)
	_, err := New(port, false)
	port.Close()
	for deadline := time.Now().Add(time.Second); runtime.NumGoroutine() > before; {
		if time.Now().After(deadline) {
			t.Errorf("%d goroutines left after New failed", runtime.NumGoroutine()-before)
			break
		}
		time.Sleep(time.Millisecond)
	}
	var me *ModemError
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want a *ModemError", err)
	}
	if me.Command != "+CPMS" || me.Code != CMESIMNotInserted || me.CMS {
		t.Errorf("ModemError = %+v", me)
	}
	if !strings.Contains(err.Error(), "SIM not inserted") {
		t.Errorf("Error() = %q, want the meaning of code 10", err.Error())
	}
}

func TestNewModemError(t *testing.T) {
	for status, want := range map[string]ModemError{
		"+CME ERROR: 10": {Command: "+X", Status: "+CME ERROR: 10", Code: 10},
		"+CMS ERROR: 500": {Command: "+X", Status: "+CMS ERROR: 500", Code: 500, CMS: true},
		"ERROR":          {Command: "+X", Status: "ERROR", Code: -1},
		"":               {Command: "+X", Status: "ERROR", Code: -1},
		"+CME ERROR: x":  {Command: "+X", Status: "+CME ERROR: x", Code: -1},
	} {
		if got := newModemError("+X", status); *got != want {
			t.Errorf("newModemError(%q) = %+v, want %+v", status, *got, want)
		}
	}
}

// A CMS code is not a CME code, so it gets no CME meaning even where the
// numbers coincide.
func TestModemErrorTextCMS(t *testing.T) {
	e := &ModemError{Command: "+CMGS", Status: "+CMS ERROR: 10", Code: 10, CMS: true}
	if got := e.Error(); got != "AT+CMGS: +CMS ERROR: 10" {
		t.Errorf("Error() = %q", got)
	}
}

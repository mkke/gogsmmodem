package gogsmmodem

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// errMockClosed is the error of a Read from a closed MockSerialPort.
var errMockClosed = errors.New("mock port closed")

// MockSerialPort is a port for tests that replays a dialogue. An entry that
// starts with "->" is the next write the port expects, and one that starts
// with "<-" is what the port answers to it.
type MockSerialPort struct {
	replay    []string
	receive   chan string
	closed    chan struct{}
	closeOnce sync.Once
}

// NewMockSerialPort returns a MockSerialPort for replay. The answers at the
// start of replay are readable at once. A write that replay does not expect
// panics.
func NewMockSerialPort(replay []string) *MockSerialPort {
	self := &MockSerialPort{
		replay:  replay,
		// Write queues all answers up to the next command while listen
		// waits for it, so the buffer bounds the answers per command.
		receive: make(chan string, 64),
		closed:  make(chan struct{}),
	}
	self.enqueueReads()
	return self
}

// Read returns the next answer of the replay, waiting for it. After Close it
// returns errMockClosed, as a serial port does.
func (self *MockSerialPort) Read(b []byte) (int, error) {
	select {
	case line := <-self.receive:
		data := []byte(line)
		copy(b, data)
		return len(data), nil
	case <-self.closed:
		return 0, errMockClosed
	}
}

func (self *MockSerialPort) enqueueReads() {
	// enqueue response(s) from replay
	for {
		if len(self.replay) == 0 || !strings.HasPrefix(self.replay[0], "<-") {
			break
		}
		i := self.replay[0][2:]
		self.receive <- i
		self.replay = self.replay[1:]
	}
}

func (self *MockSerialPort) Write(b []byte) (int, error) {
	if len(self.replay) == 0 {
		fmt.Printf("Expected: no more interactions, got: %#v", string(b))
		panic("fail")
	}
	i := self.replay[0]
	if strings.Index(i, "->") != 0 {
		fmt.Println("Replay isn't data to send:", i)
		panic("fail")
	}
	expected := i[2:]
	if expected != string(b) {
		fmt.Printf("Expected: %#v got: %#v", expected, string(b))
		panic("fail")
	}
	self.replay = self.replay[1:]
	self.enqueueReads()
	return len(b), nil
}

// Close ends a Read that waits for the replay. A second Close does nothing.
func (self *MockSerialPort) Close() error {
	self.closeOnce.Do(func() { close(self.closed) })
	return nil
}

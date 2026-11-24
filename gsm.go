// Package gogsmmodem sends and receives SMS messages through a GSM modem
// that takes AT commands in text mode. New initialises the modem on a port
// the caller has opened. The reports the modem sends without a command, such
// as a new-message notification, arrive on Modem.OOB.
//
// One goroutine per Modem reads the port and writes the commands. It stops
// on Close, on the first failed read from the port, and when a command gets
// no answer within CommandTimeout. It then closes OOB, so a caller ranging
// over OOB sees the end of the modem in each case. A command after that
// returns ErrClosed.
package gogsmmodem

import (
	"bufio"
	"errors"
	"io"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CommandTimeout bounds the wait for the answer to one command. It covers
// +CMGS, which waits for the network to take the message.
const CommandTimeout = 2 * time.Minute

// commandTimeout is the bound in force. The tests shorten it.
var commandTimeout = CommandTimeout

// Modem is a GSM modem that New has initialised. Each method sends one AT
// command and waits for its reply, so a Modem serves one caller at a time.
// OOB carries the reports the modem sends without a command: message
// notifications and service, network and signal status.
type Modem struct {
	OOB   chan Packet
	Debug bool
	port  io.ReadWriteCloser
	rx    chan Packet
	tx    chan string
	// done is closed to stop listen, and stopped is closed when it has.
	done      chan struct{}
	stopped   chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
}

// New initialises the modem on port and returns it. The caller opens port
// with the line settings the modem needs, and closes it when New returns an
// error. With debug set, every read from and write to port is logged.
func New(port io.ReadWriteCloser, debug bool) (*Modem, error) {
	if debug {
		port = LogReadWriteCloser{port}
	}
	oob := make(chan Packet, 16)
	rx := make(chan Packet)
	tx := make(chan string)
	modem := &Modem{
		OOB:     oob,
		Debug:   debug,
		port:    port,
		rx:      rx,
		tx:      tx,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	// run send/receive goroutine
	go modem.listen()

	if err := modem.init(); err != nil {
		// The port stays open for the caller to close. Closing it ends a
		// write that listen is blocked in and the read that lineChannel's
		// goroutine is blocked in, so New does not wait for them.
		modem.stop()
		return nil, err
	}
	return modem, nil
}

// stop tells listen to stop, without waiting for it.
func (self *Modem) stop() {
	self.stopOnce.Do(func() { close(self.done) })
}

// Close stops the goroutine that reads the port, closes the port and returns
// the error of closing it. It returns once that goroutine has stopped. The
// packets already on OOB stay readable, and OOB is closed after them. A
// command after Close returns ErrClosed, and a second Close returns nil.
func (self *Modem) Close() error {
	var err error
	self.closeOnce.Do(func() {
		self.stop()
		// Closing the port ends a write that listen is blocked in.
		err = self.port.Close()
	})
	<-self.stopped
	return err
}

// Commands

// GetMessage by index n from memory.
func (self *Modem) GetMessage(n int) (*Message, error) {
	packet, err := self.send("+CMGR", n)
	if err != nil {
		return nil, err
	}
	if msg, ok := packet.(Message); ok {
		return &msg, nil
	}
	return nil, errors.New("Message not found")
}

// ListMessages stored in memory. Filter should be "ALL", "REC UNREAD", "REC READ", etc.
func (self *Modem) ListMessages(filter string) (*MessageList, error) {
	packet, err := self.send("+CMGL", filter)
	if err != nil {
		return nil, err
	}
	res := MessageList{}
	if _, ok := packet.(OK); ok {
		// empty response
		return &res, nil
	}

	// The messages after the first get a deadline of their own.
	deadline := time.NewTimer(commandTimeout)
	defer deadline.Stop()
	for {
		if msg, ok := packet.(Message); ok {
			res = append(res, msg)
			if msg.Last {
				break
			}
		} else {
			return nil, errors.New("Unexpected error")
		}

		if packet, err = self.receive(deadline.C); err != nil {
			return nil, err
		}
	}
	return &res, nil
}

// SupportedStorageAreas returns the message storage areas the modem offers
// for reading, writing and receiving. It returns an error when the modem
// answers with something other than that list.
func (self *Modem) SupportedStorageAreas() (*StorageAreas, error) {
	packet, err := self.send("+CPMS", "?")
	if err != nil {
		return nil, err
	}
	if msg, ok := packet.(StorageAreas); ok {
		return &msg, nil
	}
	return nil, errors.New("Unexpected response type")
}

// DeleteMessage deletes the message at index n from the storage area.
func (self *Modem) DeleteMessage(n int) error {
	_, err := self.send("+CMGD", n)
	return err
}

// SendMessage sends body as a text message to telephone. A character of body
// that the GSM 03.38 alphabet does not hold is left out.
func (self *Modem) SendMessage(telephone, body string) error {
	enc := gsmEncode(body)
	_, err := self.sendBody("+CMGS", enc, telephone)
	return err
}

// lineChannel returns the lines read from r, without their line ends and
// without empty lines. The channel is closed after the first read error, or
// when done is closed.
func lineChannel(r io.Reader, done <-chan struct{}) <-chan string {
	ret := make(chan string)
	go func() {
		defer close(ret)
		buffer := bufio.NewReader(r)
		for {
			line, err := buffer.ReadString(10)
			// A modem ends a line with CRLF, and some adapters with LF alone.
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line != "" {
				select {
				case ret <- line:
				case <-done:
					return
				}
			}
			// bufio reports ErrNoProgress after 100 reads that returned
			// no data and no error, which is what a port with a read
			// timeout returns on an idle line.
			if err != nil && err != io.ErrNoProgress {
				return
			}
		}
	}()
	return ret
}

var reQuestion = regexp.MustCompile(`AT(\+[A-Z]+)`)

func isFinalStatus(status string) bool {
	return status == "OK" ||
		status == "ERROR" ||
		strings.Contains(status, "+CMS ERROR") ||
		strings.Contains(status, "+CME ERROR")
}

func parsePacket(status, header, body string) Packet {
	if header == "" && isFinalStatus(status) {
		if status == "OK" {
			return OK{}
		} else {
			return ERROR{Status: status}
		}
	}

	ls := strings.SplitN(header, ":", 2)
	if len(ls) != 2 {
		return UnknownPacket{header, []any{}}
	}
	uargs := strings.TrimSpace(ls[1])
	args := unquotes(uargs)
	switch ls[0] {
	case "+ZUSIMR":
		// message storage unset nag, ignore
		return nil
	case "+ZPASR":
		return ServiceStatus{args[0].(string)}
	case "+ZDONR":
		return NetworkStatus{args[0].(string)}
	case "^RSSI":
		return RSSIStatus{args[0].(int)}
	case "^HCSQ":
		switch len(args) {
		case 2:
			return SignalStatus{args[0].(string), args[1].(int), 0, 0, 0}
		case 3:
			return SignalStatus{args[0].(string), args[1].(int), args[2].(int), 0, 0}
		case 4:
			return SignalStatus{args[0].(string), args[1].(int), args[2].(int), args[3].(int), 0}
		default:
			return SignalStatus{args[0].(string), args[1].(int), args[2].(int), args[3].(int), args[4].(int)}
		}
	case "+CMTI":
		return MessageNotification{args[0].(string), args[1].(int)}
	case "+CSCA":
		return SMSCAddress{args}
	case "+CMGR":
		return Message{Status: args[0].(string), Telephone: args[1].(string),
			Timestamp: parseTime(args[3].(string)), Body: body}
	case "+CMGL":
		return Message{
			Index:     args[0].(int),
			Status:    args[1].(string),
			Telephone: args[2].(string),
			Timestamp: parseTime(args[4].(string)),
			Body:      body,
			Last:      status != "",
		}
	case "+CPMS":
		s := uargs
		if strings.HasPrefix(s, "(") {
			// query response
			// ("A","B","C"),("A","B","C"),("A","B","C")
			s = strings.TrimPrefix(s, "(")
			s = strings.TrimSuffix(s, ")")
			areas := strings.SplitN(s, "),(", 3)
			return StorageAreas{
				stringsUnquotes(areas[0]),
				stringsUnquotes(areas[1]),
				stringsUnquotes(areas[2]),
			}
		} else {
			// set response
			// 0,100,0,100,0,100
			// get ints
			var iargs []int
			for _, arg := range args {
				if iarg, ok := arg.(int); ok {
					iargs = append(iargs, iarg)
				}
			}
			if len(iargs) == 6 {
				return StorageInfo{
					iargs[0], iargs[1], iargs[2], iargs[3], iargs[4], iargs[5],
				}
			} else if len(iargs) == 4 {
				return StorageInfo{
					iargs[0], iargs[1], iargs[2], iargs[3], 0, 0,
				}
			}
			break

		}
	case "":
		if status == "OK" {
			return OK{}
		} else {
			return ERROR{Status: status}
		}
	}
	return UnknownPacket{ls[0], args}
}

// listen reads the port and writes the commands from tx until done is closed
// or a read from the port fails. Each send it makes also waits on done, so
// Close never waits for a reader of rx or OOB. It closes OOB and then stopped
// when it returns.
func (self *Modem) listen() {
	defer close(self.stopped)
	defer close(self.OOB)
	in := lineChannel(self.port, self.done)
	var echo, last, header, body string
	for {
		select {
		case <-self.done:
			return
		case line, ok := <-in:
			if !ok {
				return
			}
			if line == echo {
				continue // ignore echo of command
			} else if last != "" && startsWith(line, last) {
				if header != "" {
					// first of multiple responses (eg CMGL)
					packet := parsePacket("", header, body)
					select {
					case self.rx <- packet:
					case <-self.done:
						return
					}
				}
				header = line
				body = ""
			} else if isFinalStatus(line) {
				packet := parsePacket(line, header, body)
				select {
				case self.rx <- packet:
				case <-self.done:
					return
				}
				header = ""
				body = ""
			} else if header != "" {
				// the body following a header
				body += line
			} else if line == "> " {
				// raw mode for body
			} else {
				// OOB packet
				p := parsePacket("OK", line, "")
				if p != nil {
					select {
					case self.OOB <- p:
					case <-self.done:
						return
					}
				}
			}
		case line := <-self.tx:
			m := reQuestion.FindStringSubmatch(line)
			if len(m) > 0 {
				last = m[1]
			}
			echo = strings.TrimSuffix(strings.TrimSuffix(line, "\r\n"), "\r");
			self.port.Write([]byte(line))
		}
	}
}

func formatCommand(cmd string, args ...any) string {
	line := "AT" + cmd
	if len(args) > 0 {
		line += "=" + quotes(args)
	}
	line += "\r\n"
	return line
}

// transmit hands line to listen for writing to the port. It returns
// ErrClosed when listen has stopped, and ErrTimeout at deadline.
func (self *Modem) transmit(line string, deadline <-chan time.Time) error {
	select {
	case self.tx <- line:
		return nil
	case <-self.stopped:
		return ErrClosed
	case <-deadline:
		return self.expire()
	}
}

// receive returns the next answer that listen has read. It returns ErrClosed
// when listen has stopped, and ErrTimeout at deadline.
func (self *Modem) receive(deadline <-chan time.Time) (Packet, error) {
	select {
	case packet := <-self.rx:
		return packet, nil
	case <-self.stopped:
		return nil, ErrClosed
	case <-deadline:
		return nil, self.expire()
	}
}

// expire stops listen after a command missed its deadline, and returns
// ErrTimeout. The modem's state is unknown then, and a late answer would be
// taken for the answer to the next command.
func (self *Modem) expire() error {
	self.stop()
	return ErrTimeout
}

// sendBody sends the AT command cmd with args, then body, and returns the
// modem's answer, as send does.
func (self *Modem) sendBody(cmd string, body string, args ...any) (Packet, error) {
	deadline := time.NewTimer(commandTimeout)
	defer deadline.Stop()
	if err := self.transmit(formatCommand(cmd, args...), deadline.C); err != nil {
		return nil, err
	}
	if err := self.transmit(body+"\x1a", deadline.C); err != nil {
		return nil, err
	}
	response, err := self.receive(deadline.C)
	if err != nil {
		return nil, err
	}
	if e, ok := response.(ERROR); ok {
		return response, newModemError(cmd, e.Status)
	}
	return response, nil
}

// send sends the AT command cmd with args and returns the modem's answer.
// An answer of ERROR, +CME ERROR or +CMS ERROR comes back as a *ModemError,
// and a Modem whose reader has stopped returns ErrClosed. Without an answer
// within commandTimeout it returns ErrTimeout and stops the reader.
func (self *Modem) send(cmd string, args ...any) (Packet, error) {
	deadline := time.NewTimer(commandTimeout)
	defer deadline.Stop()
	if err := self.transmit(formatCommand(cmd, args...), deadline.C); err != nil {
		return nil, err
	}
	response, err := self.receive(deadline.C)
	if err != nil {
		return nil, err
	}
	if e, ok := response.(ERROR); ok {
		return response, newModemError(cmd, e.Status)
	}
	return response, nil
}

func (self *Modem) init() error {
	// clear settings
	if _, err := self.send("Z"); err != nil {
		return err
	}
	log.Println("Reset")
	// turn off echo
	if _, err := self.send("E0"); err != nil {
		return err
	}
	log.Println("Echo off")
	// use combined storage (MT)
	msg, err := self.send("+CPMS", "SM", "SM", "SM")
	if err != nil {
		msg, err = self.send("+CPMS", "SM", "SM")
		if err != nil {
			return err
		}
	}
	sinfo := msg.(StorageInfo)
	log.Printf("Set SMS Storage: %d/%d used\n", sinfo.UsedSpace1, sinfo.MaxSpace1)
	// set SMS text mode - easiest to implement. Ignore response which is
	// often a benign error.
	self.send("+CMGF", 1)

	log.Println("Set SMS routing mode");
	r, err := self.send("+CNMI", 0, 1, 0, 1, 1);
	if err != nil {
		return err
	}

	log.Println("Set SMS text mode")
	// get SMSC
	// the modem complains if SMSC has not been set, but stores it correctly, so
	// query for stored value, then send a set from the query response.
	r, err = self.send("+CSCA?")
	if err != nil {
		return err
	}
	smsc := r.(SMSCAddress)
	log.Println("Got SMSC:", smsc.Args)
	r, err = self.send("+CSCA", smsc.Args...)
	if err != nil {
		return err
	}
	log.Println("Set SMSC to:", smsc.Args)
	return nil
}

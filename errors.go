package gogsmmodem

import (
	"errors"
	"strconv"
	"strings"
)

// ErrClosed is the error of a command after Close, or after a read from the
// port failed.
var ErrClosed = errors.New("gogsmmodem: modem closed")

// ErrTimeout is the error of a command the modem did not answer within
// CommandTimeout. The Modem then stops reading the port and closes OOB. The
// port stays open until Close.
var ErrTimeout = errors.New("gogsmmodem: no answer from the modem")

// CME error codes of 3GPP TS 27.007 section 9.2 that a caller tells apart.
const (
	CMESIMNotInserted = 10
	CMESIMPINRequired = 11
	CMESIMFailure     = 13
	CMESIMBusy        = 14
	CMESIMWrong       = 15
)

// cmeMeanings names the CME codes the error text spells out.
var cmeMeanings = map[int]string{
	CMESIMNotInserted: "SIM not inserted",
	CMESIMPINRequired: "SIM PIN required",
	CMESIMFailure:     "SIM failure",
	CMESIMBusy:        "SIM busy",
	CMESIMWrong:       "SIM wrong",
}

// ModemError is the error of a command the modem answered with ERROR,
// +CME ERROR: <n> or +CMS ERROR: <n>. Command is the AT command without its
// arguments, such as "+CPMS". Status is the modem's final line. Code is the
// number of a +CME or +CMS answer and -1 for a plain ERROR. CMS tells the
// two kinds apart.
type ModemError struct {
	Command string
	Status  string
	Code    int
	CMS     bool
}

// Error returns the command and the modem's answer, such as
// "AT+CPMS: +CME ERROR: 10 (SIM not inserted)".
func (e *ModemError) Error() string {
	s := "AT" + e.Command + ": " + e.Status
	if meaning, ok := cmeMeanings[e.Code]; ok && !e.CMS {
		s += " (" + meaning + ")"
	}
	return s
}

// newModemError returns the ModemError of command for the final status
// line. A status without a number after "+CME ERROR:" or "+CMS ERROR:", and
// a plain ERROR, give Code -1.
func newModemError(command, status string) *ModemError {
	e := &ModemError{Command: command, Status: status, Code: -1}
	for _, prefix := range []string{"+CME ERROR:", "+CMS ERROR:"} {
		if strings.HasPrefix(status, prefix) {
			e.CMS = prefix == "+CMS ERROR:"
			if n, err := strconv.Atoi(strings.TrimSpace(status[len(prefix):])); err == nil {
				e.Code = n
			}
			break
		}
	}
	if e.Status == "" {
		e.Status = "ERROR"
	}
	return e
}

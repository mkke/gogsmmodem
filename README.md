## gogsmmodem: Go library for GSM modems

[![Build Status](https://travis-ci.org/barnybug/gogsmmodem.svg?branch=master)](https://travis-ci.org/barnybug/gogsmmodem)

Go library for the sending and receiving SMS messages through a GSM modem.

### Tested devices
- ZTE MF110/MF627/MF636

### Installation
Run:

    go get github.com/mkke/gogsmmodem

### Usage
The caller opens the serial port, with any library that returns an
`io.ReadWriteCloser`, and passes it to `New`. Example:

```go

package main

import (
    "fmt"

    "go.bug.st/serial"

    "github.com/mkke/gogsmmodem"
)

func main() {
    port, err := serial.Open("/dev/ttyUSB1", &serial.Mode{BaudRate: 115200})
    if err != nil {
        panic(err)
    }
    modem, err := gogsmmodem.New(port, true)
    if err != nil {
        port.Close()
        panic(err)
    }

    for packet := range modem.OOB {
        fmt.Printf("%#v\n", packet)
        switch p := packet.(type) {
        case gogsmmodem.MessageNotification:
            fmt.Println("Message notification:", p)
            msg, err := modem.GetMessage(p.Index)
            if err == nil {
                fmt.Printf("Message from %s: %s\n", msg.Telephone, msg.Body)
                modem.DeleteMessage(p.Index)
            }
        }
    }
}
```

### Changelog
0.3.0

- A command the modem does not answer within `CommandTimeout` returns
  `ErrTimeout`, and the `Modem` stops reading the port and closes `OOB`.

0.2.0

- `New` takes a port the caller has opened.
- A command the modem refuses returns a `*ModemError` with the `+CME` or
  `+CMS` code.
- `Close` stops the goroutine that reads the port, and a second `Close`
  returns nil.
- A command after `Close`, or after a failed read from the port, returns
  `ErrClosed`.
- A failed read from the port closes `OOB`.

0.1.0

- First release

// Example of receiving messages.
package main

import (
	"fmt"
	"log"

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
		log.Printf("Received: %#v\n", packet)
		switch p := packet.(type) {
		case gogsmmodem.MessageNotification:
			log.Println("Message notification:", p)
			msg, err := modem.GetMessage(p.Index)
			if err == nil {
				fmt.Printf("Message from %s: %s\n", msg.Telephone, msg.Body)
				modem.DeleteMessage(p.Index)
			}
		}
	}
}

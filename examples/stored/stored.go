// Example of retrieving stored messages.
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

	li, _ := modem.ListMessages("ALL")
	fmt.Printf("%d stored messages\n", len(*li))
	for _, msg := range *li {
		fmt.Println(msg.Index, msg.Status, msg.Body)
	}
}

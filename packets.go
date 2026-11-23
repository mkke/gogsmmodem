package gogsmmodem

import "time"

// Packet is an answer or a report that the modem sent. Its dynamic type is
// one of the types in this file.
type Packet any

// ServiceStatus is a +ZPASR report: the service the modem has.
type ServiceStatus struct {
	Status string
}

// NetworkStatus is a +ZDONR report: the network the modem is on.
type NetworkStatus struct {
	Network string
}

// SignalStatus is a ^HCSQ report: the system mode and its signal values.
type SignalStatus struct {
	Signal string
	RSSI int
	RSSP int
	SINR int
	RSRQ int
}

// RSSIStatus is a ^RSSI report: the received signal strength.
type RSSIStatus struct {
	RSSI int
}

// MessageNotification is a +CMTI report: a new message at Index in Storage.
type MessageNotification struct {
	Storage string
	Index   int
}

// SMSCAddress is the +CSCA answer: the service centre address.
type SMSCAddress struct {
	Args []any
}

// Message is a +CMGR or +CMGL answer: one stored message.
type Message struct {
	Index     int
	Status    string
	Telephone string
	Timestamp time.Time
	Body      string
	Last      bool
}

// StorageAreas is the +CPMS=? answer: the storage areas the modem offers.
type StorageAreas struct {
	Received []string
	Sent     []string
	New      []string
}

// StorageInfo is the answer of +CPMS with areas: the used and the maximum
// space of each area.
type StorageInfo struct {
	UsedSpace1, MaxSpace1, UsedSpace2, MaxSpace2, UsedSpace3, MaxSpace3 int
}

// MessageList is the +CMGL answer: the stored messages.
type MessageList []Message

// OK is the answer of a command the modem accepted.
type OK struct{}

// ERROR is the answer of a command the modem refused.
type ERROR struct {
	// Status is the modem's final line: ERROR, +CME ERROR: <n> or
	// +CMS ERROR: <n>.
	Status string
}

// UnknownPacket is a line the package does not parse: its command and its
// arguments.
type UnknownPacket struct {
	Command string
	Args    []any
}

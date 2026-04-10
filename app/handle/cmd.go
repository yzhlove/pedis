package handle

type Cmd uint16

const (
	UnknownCmd   Cmd = 0x00
	AuthCmd      Cmd = 0x01
	HeartbeatCmd Cmd = 0x02
)

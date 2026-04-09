package handle

type Cmd uint16

const (
	AuthCmd      Cmd = 0x01
	HeartbeatCmd Cmd = 0x02
)

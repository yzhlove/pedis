package codec

// Cmd is the two-byte command code used in the custom framing protocol.
type Cmd uint16

const (
	UnknownCmd   Cmd = 0x00
	AuthCmd      Cmd = 0x01
	HeartbeatCmd Cmd = 0x02
	HelloCmd     Cmd = 0x03
	FreeCmd      Cmd = 0x0F
)

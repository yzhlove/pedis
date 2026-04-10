package packet

import (
	"encoding/binary"
	"fmt"
)

var (
	errPacketTooLarge = fmt.Errorf("packet: packet too large")
)

type Packet []byte

func (p Packet) Cmd() uint16 {
	if len(p) > 2 {
		return binary.BigEndian.Uint16(p[:2])
	}
	return 0
}

func (p Packet) Payload() []byte {
	if len(p) > 2 {
		return p[2:]
	}
	return nil
}

func (p Packet) Pack(cmd uint16) []byte {
	data := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(data[:2], cmd)
	copy(data[2:], p)
	return data
}

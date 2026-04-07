package packet

import (
	"encoding/binary"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"
)

var (
	errPacketTooLarge = fmt.Errorf("packet: packet too large")
)

type Packet []byte

func (p Packet) pack() []byte {

	return nil
}

func Pack(command uint16, msg proto.Message) error {

	m, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("packet: %v", err)
	}

	if len(m) > math.MaxUint16 {
		return errPacketTooLarge
	}

	buf := make([]byte, 4+len(m))
	binary.BigEndian.PutUint16(buf, len())
	copy(buf[2:], m)

}

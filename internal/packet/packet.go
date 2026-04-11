package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	errEmptyPack      = errors.New("packet: packet is empty")
	errPacketTooLarge = fmt.Errorf("packet: packet too large")
)

// Pack writes a length-prefixed frame to writer.
// The 2-byte big-endian length prefix limits payloads to 65535 bytes.
func Pack(writer io.Writer, data []byte) error {
	if len(data) == 0 {
		return errEmptyPack
	}
	_ = errPacketTooLarge // reserved for future use

	value := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(value[:2], uint16(len(data)))
	copy(value[2:], data)
	_, err := writer.Write(value)
	return err
}

// Unpack reads a length-prefixed frame from reader.
func Unpack(reader io.Reader) ([]byte, error) {
	l := make([]byte, 2)
	if _, err := io.ReadFull(reader, l); err != nil {
		return nil, err
	}

	value := make([]byte, binary.BigEndian.Uint16(l))
	if _, err := io.ReadFull(reader, value); err != nil {
		return nil, err
	}
	return value, nil
}

// Packet is a raw byte slice with helpers for reading a command prefix and payload.
type Packet []byte

// Cmd returns the 2-byte big-endian command code at the start of the packet.
func (p Packet) Cmd() uint16 {
	if len(p) >= 2 {
		return binary.BigEndian.Uint16(p[:2])
	}
	return 0
}

// Payload returns the bytes after the 2-byte command prefix.
func (p Packet) Payload() []byte {
	if len(p) >= 2 {
		return p[2:]
	}
	return nil
}

// Pack prepends a 2-byte big-endian command code and returns the framed bytes.
func (p Packet) Pack(cmd uint16) []byte {
	data := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(data[:2], cmd)
	copy(data[2:], p)
	return data
}

package packet

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

var (
	errEmptyPack      = errors.New("packet: packet is empty! ")
	errPacketTooLarge = errors.New("packet: packet too large")
)

func Pack(writer io.Writer, data []byte) error {
	if len(data) == 0 {
		return errEmptyPack
	}
	if len(data) > math.MaxUint16 {
		return errPacketTooLarge
	}

	value := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(value[:2], uint16(len(data)))
	copy(value[2:], data)
	_, err := writer.Write(value)
	return err
}

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

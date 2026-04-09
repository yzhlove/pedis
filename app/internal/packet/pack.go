package packet

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	errEmptyPack = errors.New("packet: packet is empty! ")
)

func Pack(writer io.Writer, data []byte) error {
	if len(data) == 0 {
		return errEmptyPack
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

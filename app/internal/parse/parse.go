package parse

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func GetObject(reader io.Reader, callback func(obj Object) error) error {
	br := bufio.NewReader(reader)

	typeByte, err := br.ReadByte()
	if err != nil {
		return err
	}

	line, err := br.ReadString(LF)
	if err != nil {
		return err
	}
	line = strings.TrimSuffix(line, Sep)

	switch typeByte {
	case '+':
		s := GetStatus()
		defer FreeStatus(s)
		s.Build(line)
		if callback != nil {
			return callback(s)
		}
	case '-':
		e := GetError()
		defer FreeError(e)
		e.Build(line)
		if callback != nil {
			return callback(e)
		}
	case ':':
		i := GetInteger()
		defer FreeInteger(i)
		if err = i.BuildString(line); err != nil {
			return err
		}
		if callback != nil {
			return callback(i)
		}
	case '$':
		b := GetBulk()
		defer FreeBulk(b)
		if err = b.Build(line, br); err != nil {
			return err
		}
		if callback != nil {
			return callback(b)
		}
	case '*':
		a := GetArrBulk()
		defer FreeArrBulk(a)
		if err = a.Build(line, br); err != nil {
			return err
		}
		if callback != nil {
			return callback(a)
		}
	default:
		return fmt.Errorf("parse: unknown RESP type %q", typeByte)
	}
	return nil
}

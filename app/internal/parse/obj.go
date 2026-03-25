package parse

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/yzhlove/peids/app/helper"
)

var (
	errBulkTrial  = errors.New("parse: bulk string missing CRLF")
	errBulkHeader = errors.New("parse: bulk string missing header")
)

type Status struct {
	value string
}

func (s *Status) Build(value string) {
	s.value = value
}

func (s *Status) Type() Type {
	return StatusType
}

func (s *Status) Get() string {
	return s.value
}

func (s *Status) ToBytes() []byte {
	buf := helper.Get1KBBytes()
	defer helper.FreeBytes(buf)
	b := bytes.NewBuffer(buf)
	b.Reset()
	b.WriteByte('+')
	b.WriteString(s.value)
	b.WriteString(Sep)
	return b.Bytes()
}

type Error struct {
	value string
}

func (e *Error) Type() Type {
	return ErrorType
}

func (e *Error) Build(value string) {
	e.value = value
}

func (e *Error) BuildErr(err error) {
	e.value = err.Error()
}

func (e *Error) GetErr() error {
	return errors.New(e.value)
}

func (e *Error) Get() string {
	return e.value
}

func (e *Error) ToBytes() []byte {
	buf := helper.Get1KBBytes()
	defer helper.FreeBytes(buf)
	b := bytes.NewBuffer(buf)
	b.Reset()
	b.WriteByte('-')
	b.WriteString(e.value)
	b.WriteString(Sep)
	return b.Bytes()
}

type Integer struct {
	value int64
}

func (i *Integer) Type() Type {
	return IntegerType
}

func (i *Integer) Build(value int64) {
	i.value = value
}

func (i *Integer) Get() int64 {
	return i.value
}

func (i *Integer) BuildString(value string) error {
	res, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	i.value = res
	return nil
}

func (i *Integer) ToBytes() []byte {
	buf := helper.Get1KBBytes()
	defer helper.FreeBytes(buf)
	b := bytes.NewBuffer(buf)
	b.Reset()
	b.WriteByte(':')
	b.WriteString(strconv.FormatInt(i.value, 10))
	b.WriteString(Sep)
	return b.Bytes()
}

type Bulk struct {
	value string
}

func (b *Bulk) Type() Type {
	return BulkType
}

func (b *Bulk) Get() string {
	return b.value
}

func (b *Bulk) Build(length string, reader *bufio.Reader) error {
	n, err := strconv.Atoi(length)
	if err != nil {
		return err
	}
	// null bulk string: $-1\r\n
	if n == -1 {
		b.value = ""
		return nil
	}
	buf := make([]byte, n+2) // +2 for trailing \r\n
	if _, err = io.ReadFull(reader, buf); err != nil {
		return err
	}
	if buf[n] != '\r' || buf[n+1] != '\n' {
		return errBulkTrial
	}
	b.value = string(buf[:n])
	return nil
}

func (b *Bulk) BuildString(value string) {
	b.value = value
}

func (b *Bulk) ToBytes() []byte {
	data := helper.Get1KBBytes()
	defer helper.FreeBytes(data)
	buf := bytes.NewBuffer(data)
	buf.Reset()
	buf.WriteByte('$')
	buf.WriteString(strconv.Itoa(len(b.value)))
	buf.WriteString(Sep)
	buf.WriteString(b.value)
	buf.WriteString(Sep)
	return buf.Bytes()
}

type ArrBulk struct {
	value []string
}

func (a *ArrBulk) Type() Type {
	return ArrBulkType
}

func (a *ArrBulk) Get() []string {
	return a.value
}

func (a *ArrBulk) Build(length string, reader *bufio.Reader) error {
	n, err := strconv.Atoi(length)
	if err != nil {
		return err
	}
	// null bulk string: $-1\r\n
	if n == -1 {
		a.value = []string{}
		return nil
	}
	a.value = make([]string, n)
	b := GetBulk()
	defer FreeBulk(b)

	for i := 0; i < n; i++ {
		c, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if c != '$' {
			return errBulkHeader
		}
		text, err := reader.ReadString(LF)
		if err != nil {
			return err
		}
		if err = b.Build(strings.TrimSuffix(text, Sep), reader); err != nil {
			return err
		}
		a.value[i] = b.value
		b.value = ""
	}
	return nil
}

func (a *ArrBulk) BuildArray(strs []string) {
	a.value = strs
}

func (a *ArrBulk) ToBytes() []byte {
	data := helper.Get1KBBytes()
	defer helper.FreeBytes(data)
	buf := bytes.NewBuffer(data)
	buf.Reset()
	buf.WriteByte('*')
	buf.WriteString(strconv.Itoa(len(a.value)))
	buf.WriteString(Sep)
	for _, v := range a.value {
		buf.WriteString("$")
		buf.WriteString(strconv.Itoa(len(v)))
		buf.WriteString(Sep)
		buf.WriteString(v)
		buf.WriteString(Sep)
	}
	return buf.Bytes()
}

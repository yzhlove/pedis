package redis

import (
	"io"

	"github.com/yzhlove/pedis/internal/resp"
)

// PING sends a Redis PING command to w.
func PING(w io.Writer) (err error) {
	b := resp.GetArrBulk()
	defer resp.FreeArrBulk(b)

	b.BuildArray([]string{"PING"})
	_, err = w.Write(b.ToBytes())
	return
}

func OK(w io.Writer) (err error) {
	s := resp.GetStatus()
	defer resp.FreeStatus(s)

	s.Build("OK")
	_, err = w.Write(s.ToBytes())
	return
}

func PONG(w io.Writer) (err error) {
	s := resp.GetStatus()
	defer resp.FreeStatus(s)

	s.Build("PONG")
	_, err = w.Write(s.ToBytes())
	return
}

func ErrNoAuth(w io.Writer) (err error) {
	e := resp.GetError()
	defer resp.FreeError(e)

	e.Build("ERR NOAUTH Authentication required")
	_, err = w.Write(e.ToBytes())
	return
}

func ErrResp2Only(w io.Writer) (err error) {
	e := resp.GetError()
	defer resp.FreeError(e)
	e.Build("ERR This server only supports RESP2")
	_, err = w.Write(e.ToBytes())
	return
}

func ErrInvalidCommand(w io.Writer) (err error) {
	e := resp.GetError()
	defer resp.FreeError(e)
	e.Build("ERR invalid command")
	_, err = w.Write(e.ToBytes())
	return
}

func ErrInvalidArguments(w io.Writer) (err error) {
	e := resp.GetError()
	defer resp.FreeError(e)
	e.Build("ERR invalid arguments")
	_, err = w.Write(e.ToBytes())
	return
}

func ErrWrap(w io.Writer, err error) error {
	e := resp.GetError()
	defer resp.FreeError(e)
	e.Build(err.Error())
	_, erro := w.Write(e.ToBytes())
	return erro
}

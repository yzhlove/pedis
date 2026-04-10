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

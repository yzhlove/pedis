package stdedis

import (
	"io"

	"github.com/yzhlove/peids/app/internal/parse"
)

func PING(w io.Writer) (err error) {
	b := parse.GetArrBulk()
	defer parse.FreeArrBulk(b)

	b.BuildArray([]string{"PING"})
	_, err = w.Write(b.ToBytes())
	return
}

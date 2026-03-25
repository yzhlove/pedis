package stdedis

import (
	"io"

	"github.com/yzhlove/peids/app/internal/parse"
)

func PING(w io.Writer) (err error) {
	b := parse.GetBulk()
	defer parse.FreeBulk(b)

	b.BuildString("PING")
	_, err = w.Write(b.ToBytes())
	return
}

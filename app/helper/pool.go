package helper

import (
	"github.com/bytedance/gopkg/lang/mcache"
)

func Get1KBBytes() []byte {
	return mcache.Malloc(1024)
}

func FreeBytes(data []byte) {
	clear(data)
	mcache.Free(data)
}

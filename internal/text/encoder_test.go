package text

import (
	"testing"
	"time"

	"github.com/yzhlove/pedis/internal/config"
)

func Test_Text(t *testing.T) {

	cfg := &config.Config{
		CharacterSet: "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@#$%*!+=/?",
		TimeSeed:     "psMrfhLXrUxbdyYdzzaAjQLr8mDwuu0c",
	}
	encoder := New(cfg)
	if err := encoder.Apply(); err != nil {
		t.Fatal(err)
	}

	ts := time.Now().UnixNano()
	s := Encode(uint64(ts))

	t.Log("timestamp string is => ", s)

	n, err := Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(ts) != n {
		t.Fatal("timestamp not equal")
	}
}

package conn

import (
	"testing"
	"time"

	"github.com/yzhlove/pedis/internal/config"
)

func Test_Redis(t *testing.T) {
	cc := NewRedis(&config.Config{
		CliRedisHost: "127.0.0.1",
		CliRedisPort: "6379",
	})
	if err := cc.Connect(time.Second * 5); err != nil {
		t.Fatal(err)
	}

	if err := cc.Heartbeat(); err != nil {
		t.Fatal(err)
	}
	t.Log("ok")
}

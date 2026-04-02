package conn

import (
	"errors"
	"net"
	"time"
)

type Connector interface {
	Connect(d time.Duration) error
	Heartbeat() error
	Detached() net.Conn
	Close()
	Ok() bool
}

var (
	readTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
)

var (
	errRedisHeartbeatType    = errors.New("redis: heartbeat object type error! ")
	errRedisHeartbeatCommand = errors.New("redis: heartbeat command error! ")
)

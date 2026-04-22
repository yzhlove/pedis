package conn

import (
	"errors"
	"net"
	"time"
)

// Connector manages the lifecycle of a single outbound connection.
type Connector interface {
	Connect(d time.Duration) error
	Heartbeat() error
	Detached() net.Conn
	Close()
	Ok() bool
}

var (
	errRedisHeartbeatType    = errors.New("redis: heartbeat object type error")
	errRedisHeartbeatCommand = errors.New("redis: heartbeat command error")
)

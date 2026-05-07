package config

import (
	"net"
	"time"
)

const (
	HeartbeatInterval = 2 * time.Minute
	TakeTimeout       = 30 * time.Second
)

const (
	ReadTimeout  = 10 * time.Second
	WriteTimeout = 10 * time.Second
	RWTimeout    = 15 * time.Second
)

const (
	MaxClientConns = 12
)

func ReadConnTimeout(conn net.Conn, fn func() error) error {
	if fn != nil {
		conn.SetReadDeadline(time.Now().Add(ReadTimeout))
		defer conn.SetReadDeadline(time.Time{})
		return fn()
	}
	return nil
}

func WriteConnTimeout(conn net.Conn, fn func() error) error {
	if fn != nil {
		conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
		defer conn.SetWriteDeadline(time.Time{})
		return fn()
	}
	return nil
}

func RWConnTimeout(conn net.Conn, fn func() error) error {
	if fn != nil {
		conn.SetDeadline(time.Now().Add(RWTimeout))
		defer conn.SetDeadline(time.Time{})
		return fn()
	}
	return nil
}

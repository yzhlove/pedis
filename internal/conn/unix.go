package conn

import (
	"net"
	"os"
	"time"

	"github.com/yzhlove/pedis/internal/codec"
	"github.com/yzhlove/pedis/internal/config"
)

type unixConn struct {
	cfg  *config.Config
	conn net.Conn
	cli  codec.ClientCodec
}

func NewUnix(c *config.Config) Connector {
	return &unixConn{cfg: c}
}

func (u *unixConn) rwTimeout(fn func() error) error {
	if fn == nil {
		return nil
	}
	u.conn.SetDeadline(time.Now().Add(rwTimeout))
	defer u.conn.SetDeadline(time.Time{})
	return fn()
}

func (u *unixConn) Ok() bool { return u.conn != nil }

func (u *unixConn) Connect(d time.Duration) error {
	if _, err := os.Stat(u.cfg.UnixSocket); err != nil {
		u.conn = nil
		return err
	}
	cc, err := net.DialTimeout("unix", u.cfg.UnixSocket, d)
	if err != nil {
		u.conn = nil
		return err
	}

	tc, err := codec.NewClient(u.cfg.CliName)
	if err != nil {
		u.conn = nil
		return err
	}

	if err = u.rwTimeout(func() error { return codec.Auth(tc, cc) }); err != nil {
		u.conn = nil
		return err
	}

	u.cli = tc
	u.conn = cc
	return nil
}

func (u *unixConn) Heartbeat() error {
	if err := u.rwTimeout(func() error { return codec.Heartbeat(u.cli, u.conn) }); err != nil {
		u.conn = nil
		u.cli = nil
		return err
	}
	return nil
}

func (u *unixConn) Detached() net.Conn {
	if u.conn == nil {
		return nil
	}

	if err := u.rwTimeout(func() error { return codec.Free(u.cli, u.conn) }); err != nil {
		u.conn = nil
		u.cli = nil
		return nil
	}

	cc := u.conn
	u.conn = nil
	u.cli = nil
	return cc
}

func (u *unixConn) Close() {
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
		u.cli = nil
	}
}

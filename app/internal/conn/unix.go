package conn

import (
	"net"
	"os"
	"time"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/handle/client"
)

type unix struct {
	cfg  *config.Config
	conn net.Conn
	cli  client.Codec
}

func NewUnix(c *config.Config) Connector {
	return &unix{
		cfg: c,
	}
}

func (u *unix) rwTimeout(fn func() error) error {
	if fn != nil {
		u.conn.SetDeadline(time.Now().Add(rwTimeout))
		defer u.conn.SetDeadline(time.Time{})
		return fn()
	}
	return nil
}

func (u *unix) Ok() bool {
	return u.conn != nil
}

func (u *unix) Connect(d time.Duration) (err error) {
	if _, err = os.Stat(u.cfg.UnixSocket); err != nil {
		u.conn = nil
		return err
	}
	cc, err := net.DialTimeout("unix", u.cfg.UnixSocket, d)
	if err != nil {
		u.conn = nil
		return err
	}

	tc, err := client.New(u.cfg)
	if err != nil {
		u.conn = nil
		return err
	}

	if err = u.rwTimeout(func() error { return client.Auth(tc, cc) }); err != nil {
		u.conn = nil
		return err
	}

	u.cli = tc
	u.conn = cc
	return
}

func (u *unix) Heartbeat() (err error) {
	if err = u.rwTimeout(func() error { return client.Heartbeat(u.cli, u.conn) }); err != nil {
		u.conn = nil
		u.cli = nil
		return err
	}
	return
}

func (u *unix) Detached() net.Conn {
	if u.conn == nil {
		return nil
	}
	cc := u.conn
	u.conn = nil
	u.cli = nil
	return cc
}

func (u *unix) Close() {
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
		u.cli = nil
	}
}

package conn

import (
	"net"
	"os"
	"time"
)

type unix struct {
	path string
	conn net.Conn
}

func NewUnix(path string) Connector {
	return &unix{
		path: path,
	}
}

func (u *unix) Ok() bool {
	return u.conn != nil
}

func (u *unix) Connect(d time.Duration) (err error) {
	if _, err = os.Stat(u.path); err != nil {
		u.conn = nil
		return err
	}
	cc, err := net.DialTimeout("unix", u.path, d)
	if err != nil {
		u.conn = nil
		return err
	}
	u.conn = cc
	return
}

func (u *unix) Heartbeat() error {

	return nil
}

func (u *unix) Detached() net.Conn {
	if u.conn == nil {
		return nil
	}
	cc := u.conn
	u.conn = nil
	return cc
}

func (u *unix) Close() {
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
}

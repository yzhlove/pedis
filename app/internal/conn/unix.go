package conn

import (
	"net"
	"os"
	"time"
)

type Unix struct {
	path string
	conn net.Conn
}

func NewUnix(path string) Connector {
	return &Unix{
		path: path,
	}
}

func (u *Unix) Ok() bool {
	return u.conn != nil
}

func (u *Unix) Connect(d time.Duration) (err error) {
	defer func() {
		if err != nil {
			u.conn = nil
		}
	}()
	if _, err = os.Stat(u.path); err != nil {
		return err
	}
	cc, err := net.DialTimeout("unix", u.path, d)
	if err != nil {
		return err
	}
	u.conn = cc
	return
}

func (u *Unix) Heartbeat() error {

	return nil
}

func (u *Unix) Detached() net.Conn {
	if u.conn == nil {
		return nil
	}
	cc := u.conn
	u.conn = nil
	return cc
}

func (u *Unix) Close() {
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
}

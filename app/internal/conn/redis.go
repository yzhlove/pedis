package conn

import (
	"net"
	"time"

	"github.com/yzhlove/peids/app/internal/parse"
	"github.com/yzhlove/peids/app/internal/stdedis"
)

type redis struct {
	host string
	port string
	conn net.Conn
}

func NewRedis(host, port string) Connector {
	return &redis{
		host: host,
		port: port,
	}
}

func (r *redis) readTimeout(fn func() error) error {
	if fn != nil {
		r.conn.SetReadDeadline(time.Now().Add(readTimeout))
		defer r.conn.SetReadDeadline(time.Time{})
		return fn()
	}
	return nil
}

func (r *redis) writeTimeout(fn func() error) error {
	if fn != nil {
		r.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		defer r.conn.SetWriteDeadline(time.Time{})
		return fn()
	}
	return nil
}

func (r *redis) Ok() bool {
	return r.conn != nil
}

func (r *redis) Connect(d time.Duration) (err error) {
	cc, err := net.DialTimeout("tcp", net.JoinHostPort(r.host, r.port), d)
	if err != nil {
		r.conn = nil
		return err
	}
	r.conn = cc
	return
}

func (r *redis) Heartbeat() (err error) {
	defer func() {
		if err != nil {
			r.conn = nil
		}
	}()

	if err = r.writeTimeout(func() error {
		return stdedis.PING(r.conn)
	}); err != nil {
		return err
	}

	return r.readTimeout(func() error {
		return parse.GetObject(r.conn, func(obj parse.Object) error {
			if obj.Type() != parse.StatusType {
				return errRedisHeartbeatType
			}
			if obj.(*parse.Status).Get() != "PONG" {
				return errRedisHeartbeatCommand
			}
			return nil
		})
	})
}

func (r *redis) Detached() net.Conn {
	if r.conn != nil {
		cc := r.conn
		r.conn = nil
		return cc
	}
	return nil
}

func (r *redis) Close() {
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}

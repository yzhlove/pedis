package conn

import (
	"net"
	"time"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/redis"
	"github.com/yzhlove/pedis/internal/resp"
)

type redisConn struct {
	cfg  *config.Config
	conn net.Conn
}

func NewRedis(cfg *config.Config) Connector {
	return &redisConn{cfg: cfg}
}

func (r *redisConn) readTimeout(fn func() error) error {
	if fn == nil {
		return nil
	}
	r.conn.SetReadDeadline(time.Now().Add(readTimeout))
	defer r.conn.SetReadDeadline(time.Time{})
	return fn()
}

func (r *redisConn) writeTimeout(fn func() error) error {
	if fn == nil {
		return nil
	}
	r.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	defer r.conn.SetWriteDeadline(time.Time{})
	return fn()
}

func (r *redisConn) Ok() bool { return r.conn != nil }

func (r *redisConn) Connect(d time.Duration) error {
	cc, err := net.DialTimeout("tcp", net.JoinHostPort(r.cfg.CliRedisHost, r.cfg.CliRedisPort), d)
	if err != nil {
		r.conn = nil
		return err
	}
	r.conn = cc
	return nil
}

func (r *redisConn) Heartbeat() (err error) {
	defer func() {
		if err != nil {
			r.conn = nil
		}
	}()

	if err = r.writeTimeout(func() error {
		return redis.PING(r.conn)
	}); err != nil {
		return err
	}

	return r.readTimeout(func() error {
		return resp.GetObject(r.conn, func(obj resp.Object) error {
			if obj.Type() != resp.StatusType {
				return errRedisHeartbeatType
			}
			if obj.(*resp.Status).Get() != "PONG" {
				return errRedisHeartbeatCommand
			}
			return nil
		})
	})
}

func (r *redisConn) Detached() net.Conn {
	if r.conn != nil {
		cc := r.conn
		r.conn = nil
		return cc
	}
	return nil
}

func (r *redisConn) Close() {
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
}

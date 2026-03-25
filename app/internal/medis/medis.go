package medis

import (
	"errors"
	"net"
	"time"

	"github.com/yzhlove/peids/app/internal/parse"
	"github.com/yzhlove/peids/app/internal/stdedis"
)

var (
	errHeartbeatObjType  = errors.New("medis: heartbeat object type error! ")
	errHeartbeatObjValue = errors.New("medis: heartbeat command error! ")
)

type RedisManager interface {
	Connect(d time.Duration) error
	Heartbeat() error
	TcpConn() net.Conn
	Close()
}

type medis struct {
	host string
	port string
	conn net.Conn
}

func New(host, port string) RedisManager {
	return &medis{
		host: host,
		port: port,
	}
}

func (m *medis) readTimeout(fn func() error) error {
	if fn != nil {
		m.conn.SetReadDeadline(time.Now().Add(time.Second * 10))
		defer m.conn.SetReadDeadline(time.Time{})
		return fn()
	}
	return nil
}

func (m *medis) writeTimeout(fn func() error) error {
	if fn != nil {
		m.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
		defer m.conn.SetWriteDeadline(time.Time{})
		return fn()
	}
	return nil
}

func (m *medis) Connect(d time.Duration) (err error) {
	cc, err := net.DialTimeout("tcp", net.JoinHostPort(m.host, m.port), d)
	if err != nil {
		return err
	}
	m.conn = cc
	return
}

func (m *medis) Heartbeat() error {
	if err := m.writeTimeout(func() error {
		return stdedis.PING(m.conn)
	}); err != nil {
		return err
	}

	return m.readTimeout(func() error {
		return parse.GetObject(m.conn, func(obj parse.Object) error {
			if obj.Type() != parse.StatusType {
				return errHeartbeatObjType
			}
			if obj.(*parse.Status).Get() != "PONG" {
				return errHeartbeatObjValue
			}
			return nil
		})
	})
}

func (m *medis) TcpConn() net.Conn {
	return m.conn
}

func (m *medis) Close() {
	if m.conn != nil {
		m.conn.Close()
	}
}

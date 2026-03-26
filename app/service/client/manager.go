package client

import (
	"context"
	"io"
)

type manager struct {
	ctx           context.Context
	events        chan Event
	state         State
	unixWork      Worker
	redisWork     Worker
	bridge        *bridgeController
	unixUp        bool
	redisUp       bool
	detachedUnix  io.ReadWriteCloser
	detachedRedis io.ReadWriteCloser
}

func NewManager(ctx context.Context) *manager {
	m := &manager{
		ctx:    ctx,
		state:  StateNoneUp,
		events: make(chan Event, 32),
	}
	m.bridge = NewBridgeController(m)
	return m
}

func (m *manager) SendEvent(e Event) bool {
	select {
	case <-m.ctx.Done():
		return false
	default:
		select {
		case m.events <- e:
			return true
		default:
		}
	}
	return false
}

func (m *manager) Run() {
	go m.unixWork.Run()
	go m.redisWork.Run()

	for {
		select {
		case <-m.ctx.Done():
			m.stop()
			return
		case e := <-m.events:
			m.handelEvent(e)
		}
	}
}

func (m *manager) stop() {
	m.bridge.Stop()
	m.redisWork.Stop()
	m.unixWork.Stop()
	m.unixUp = false
	m.redisUp = false
	m.detachedUnix = nil
	m.detachedRedis = nil
}

func (m *manager) handelEvent(e Event) {
	switch e.typ {
	case EvRedisConnected:
		m.redisUp = true
	case EvRedisDisconnected:
		m.redisUp = false
		m.detachedRedis = nil
	case EvUnixConnected:
		m.unixUp = true
	case EvUnixDisconnected:
		m.unixUp = false
		m.detachedUnix = nil
	case EvBridgeStopped:

	}
	m.reconcile()
}

func (m *manager) desired() State {
	unixOk := m.unixConn != nil
	redisOk := m.redisConn != nil
	switch {
	case unixOk && redisOk:
		return StateBridging
	case unixOk:
		return StateUnixUp
	case redisOk:
		return StateRedisUp
	default:
		return StateNoneUp
	}
}

func (m *manager) reconcile() {

	target := m.desired()
	if target == m.state {
		return
	}

	m.exit(m.state)
	m.enter(target)
	m.state = target
}

func (m *manager) exit(s State) {
	switch s {
	case StateUnixUp:
		m.unixWork.StopHeartbeat()
	case StateRedisUp:
		m.redisWork.StopHeartbeat()
	case StateBridging:
		m.bridge.Stop()
	}
}

func (m *manager) enter(s State) {
	switch s {
	case StateUnixUp:
		m.unixWork.StartHeartbeat()
	case StateRedisUp:
		m.redisWork.StartHeartbeat()
	case StateBridging:
		m.unixWork.StopHeartbeat()
		m.redisWork.StopHeartbeat()
		if m.unixConn != nil && m.redisConn != nil {
			m.bridge.Start(m.unixConn, m.redisConn)
		}
	}
}

func (m *manager) heartbeat() {
	switch m.desired() {
	case StateUnixUp:
		m.unixWork.StartHeartbeat()
	case StateRedisUp:
		m.redisWork.StartHeartbeat()
	}
}

package client

import (
	"context"
	"io"

	"github.com/yzhlove/peids/app/config"
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
	bridging      bool
	detachedUnix  io.ReadWriteCloser
	detachedRedis io.ReadWriteCloser
}

func NewManager(ctx context.Context, cfg *config.Config) *manager {
	m := &manager{
		ctx:    ctx,
		state:  StateNoneUp,
		events: make(chan Event, 16),
	}
	m.bridge = NewBridgeController(m)
	m.redisWork = NewRedisWork(ctx, cfg, m)
	return m
}

func (m *manager) SendEvent(e Event) {
	select {
	case <-m.ctx.Done():
		return
	default:
		m.events <- e
	}
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
	case EvRedisConnectDetached:
		m.redisUp = false
		m.detachedRedis = e.rwc
	case EvUnixConnected:
		m.unixUp = true
	case EvUnixDisconnected:
		m.unixUp = false
		m.detachedUnix = nil
	case EvUnixConnectDetached:
		m.unixUp = false
		m.detachedUnix = e.rwc
	case EvBridgeStopped:
		m.bridging = false
		m.unixUp = false
		m.redisUp = false
		m.detachedRedis = nil
		m.detachedUnix = nil
	}
	m.reconcile()
}

func (m *manager) desired() State {
	switch {
	case m.bridging || (m.detachedRedis != nil && m.detachedUnix != nil):
		return StateBridging
	case m.redisUp && m.unixUp:
		return StatePreparingBridge
	case m.redisUp:
		return StateRedisUpOnly
	case m.unixUp:
		return StateUnixUpOnly
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
	case StateUnixUpOnly:
		m.unixWork.SendCmd(WorkerCmd{typ: CmdStopHeartbeat})
	case StateRedisUpOnly:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdStopHeartbeat})
	case StateBridging:
		m.bridge.Stop()
		m.redisWork.SendCmd(WorkerCmd{typ: CmdShutdown})
		m.unixWork.SendCmd(WorkerCmd{typ: CmdShutdown})
	}
}

func (m *manager) enter(s State) {
	switch s {
	case StateUnixUpOnly:
		m.unixWork.SendCmd(WorkerCmd{typ: CmdStartHeartbeat})
	case StateRedisUpOnly:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdStartHeartbeat})
	case StatePreparingBridge:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdDetachForBridge})
		m.unixWork.SendCmd(WorkerCmd{typ: CmdDetachForBridge})
	case StateBridging:
		if !m.bridging && m.detachedUnix != nil && m.detachedRedis != nil {
			m.bridging = true
			m.bridge.Start(m.detachedUnix, m.detachedRedis)
		}
	}
}

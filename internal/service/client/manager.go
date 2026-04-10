package client

import (
	"context"
	"io"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/conn"
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

func newManager(ctx context.Context, cfg *config.Config) *manager {
	m := &manager{
		ctx:    ctx,
		state:  NoneUp,
		events: make(chan Event, 16),
	}
	m.bridge = newBridgeController(m)
	m.unixWork = newWorker(ctx, "worker-unix", m, conn.NewUnix(cfg), workerEvents{
		connected:    UnixConnected,
		disconnected: UnixDisconnected,
		detached:     UnixConnectDetached,
	})
	m.redisWork = newWorker(ctx, "worker-redis", m, conn.NewRedis(cfg), workerEvents{
		connected:    RedisConnected,
		disconnected: RedisDisconnected,
		detached:     RedisConnectDetached,
	})
	return m
}

func (m *manager) SendEvent(e Event) {
	select {
	case <-m.ctx.Done():
		return
	case m.events <- e:
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
			m.handleEvent(e)
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

func (m *manager) handleEvent(e Event) {
	switch e.typ {
	case RedisConnected:
		m.redisUp = true
	case RedisDisconnected:
		m.redisUp = false
		m.detachedRedis = nil
	case RedisConnectDetached:
		m.redisUp = false
		m.detachedRedis = e.rwc
	case UnixConnected:
		m.unixUp = true
	case UnixDisconnected:
		m.unixUp = false
		m.detachedUnix = nil
	case UnixConnectDetached:
		m.unixUp = false
		m.detachedUnix = e.rwc
	case BridgeStopped:
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
		return Bridging
	case m.redisUp && m.unixUp:
		return PreparingBridge
	case m.redisUp:
		return RedisUpOnly
	case m.unixUp:
		return UnixUpOnly
	default:
		return NoneUp
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
	case UnixUpOnly:
		m.unixWork.SendCmd(WorkerCmd{typ: CmdStopHeartbeat})
	case RedisUpOnly:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdStopHeartbeat})
	case Bridging:
		m.bridge.Stop()
		m.redisWork.SendCmd(WorkerCmd{typ: CmdShutdown})
		m.unixWork.SendCmd(WorkerCmd{typ: CmdShutdown})
	}
}

func (m *manager) enter(s State) {
	switch s {
	case UnixUpOnly:
		m.unixWork.SendCmd(WorkerCmd{typ: CmdStartHeartbeat})
	case RedisUpOnly:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdStartHeartbeat})
	case PreparingBridge:
		m.redisWork.SendCmd(WorkerCmd{typ: CmdDetachForBridge})
		m.unixWork.SendCmd(WorkerCmd{typ: CmdDetachForBridge})
	case Bridging:
		if !m.bridging && m.detachedUnix != nil && m.detachedRedis != nil {
			m.bridging = true
			m.bridge.Start(m.detachedUnix, m.detachedRedis)
		}
	}
}

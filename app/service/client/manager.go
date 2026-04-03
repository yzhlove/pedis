package client

import (
	"context"
	"io"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/internal/conn"
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
		state:  StateNoneUp,
		events: make(chan Event, 16),
	}
	m.bridge = newBridgeController(m)
	m.unixWork = newWorker(ctx, "worker-unix", m, conn.NewUnix(cfg.UnixSocket), workerEvents{
		connected:    EvUnixConnected,
		disconnected: EvUnixDisconnected,
		detached:     EvUnixConnectDetached,
	})
	m.redisWork = newWorker(ctx, "worker-redis", m, conn.NewRedis(cfg.CliRedisHost, cfg.CliRedisPort), workerEvents{
		connected:    EvRedisConnected,
		disconnected: EvRedisDisconnected,
		detached:     EvRedisConnectDetached,
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

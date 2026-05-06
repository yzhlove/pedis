package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/yzhlove/pedis/internal/conn"
	"github.com/yzhlove/pedis/internal/log"
)

// workerEvents holds the EventTypes a worker emits at each connection lifecycle
// stage, allowing the same worker implementation to serve both unix and redis
// connections.
type workerEvents struct {
	connected    EventType
	disconnected EventType
	detached     EventType
}

type worker struct {
	name      string
	ctx       context.Context
	eventer   Eventer
	cmdCh     chan WorkerCmd
	mode      WorkerMode
	connector conn.Connector
	evts      workerEvents
}

func newWorker(ctx context.Context, name string, e Eventer, connector conn.Connector, evts workerEvents) Worker {
	return &worker{
		name:      name,
		ctx:       ctx,
		eventer:   e,
		connector: connector,
		evts:      evts,
		cmdCh:     make(chan WorkerCmd, 16),
	}
}

func (w *worker) stop() {
	if w.connector != nil {
		w.connector.Close()
		w.connector = nil
	}
}

func (w *worker) Run() {
	reconnectTicker := time.NewTicker(time.Second * 16)
	defer reconnectTicker.Stop()

	heartbeatTicker := time.NewTicker(time.Second * 5)
	defer heartbeatTicker.Stop()

	if err := w.tryConnect(); err != nil {
		log.Error("worker: first connect failed", slog.String("name", w.name), log.ErrWrap(err))
	}

	for {
		select {
		case <-w.ctx.Done():
			w.stop()
			return
		case cmd := <-w.cmdCh:
			w.handleCmd(cmd)
		case <-reconnectTicker.C:
			if w.mode == ModeDisconnected && !w.connector.Ok() {
				if err := w.tryConnect(); err != nil {
					log.Error("worker: connect failed", slog.String("name", w.name), log.ErrWrap(err))
				} else {
					log.Info("worker: connect success", slog.String("name", w.name))
				}
			}
		case <-heartbeatTicker.C:
			if w.mode == ModeHeartbeat && w.connector.Ok() {
				if err := w.connector.Heartbeat(); err != nil {
					log.Error("worker: heartbeat failed", slog.String("name", w.name), log.ErrWrap(err))
					w.closeConnWithEvent()
				} else {
					log.Info("worker: heartbeat success", slog.String("name", w.name))
				}
			}
		}
	}
}

func (w *worker) SendCmd(cmd WorkerCmd) {
	select {
	case w.cmdCh <- cmd:
	default:
		log.Error("worker: send cmd failed", slog.String("name", w.name))
	}
}

func (w *worker) handleCmd(cmd WorkerCmd) {
	switch cmd.typ {
	case CmdStartHeartbeat:
		if w.connector.Ok() && w.mode == ModeConnectedIdle {
			w.mode = ModeHeartbeat
		}
	case CmdStopHeartbeat:
		if w.connector.Ok() && w.mode == ModeHeartbeat {
			w.mode = ModeConnectedIdle
		}
	case CmdDetachForBridge:
		w.detached()
	case CmdShutdown:
		w.closeConnSilently()
	}
}

func (w *worker) closeConnWithEvent() {
	if w.connector.Ok() {
		w.connector.Close()
	}
	if w.mode != ModeDisconnected {
		w.eventer.SendEvent(Event{typ: w.evts.disconnected})
	}
	w.mode = ModeDisconnected
}

func (w *worker) closeConnSilently() {
	if w.connector.Ok() {
		w.connector.Close()
	}
	w.mode = ModeDisconnected
}

func (w *worker) tryConnect() error {
	if err := w.connector.Connect(time.Second * 5); err != nil {
		return err
	}
	w.mode = ModeConnectedIdle
	w.eventer.SendEvent(Event{typ: w.evts.connected})
	return nil
}

func (w *worker) detached() {
	if !w.connector.Ok() {
		return
	}
	cc := w.connector.Detached()
	w.mode = ModeDetached
	w.eventer.SendEvent(Event{typ: w.evts.detached, conn: cc})
}

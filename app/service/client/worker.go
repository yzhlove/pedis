package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/yzhlove/peids/app/internal/conn"
	"github.com/yzhlove/peids/app/log"
)

// workerEvents 持有 worker 在连接生命周期各阶段应发送的事件类型，
// 使同一 worker 实现可同时服务 unix 和 redis 两种连接。
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

	reconnectTicker := time.NewTicker(time.Second * 45)
	defer reconnectTicker.Stop()

	heartbeatTicker := time.NewTicker(time.Second * 15)
	defer heartbeatTicker.Stop()

	// 首先尝试连接一次
	if err := w.tryConnect(); err != nil {
		log.Error("worker: first connect failed! ", slog.String("name", w.name), log.ErrWrap(err))
	}

	for {
		select {
		case <-w.ctx.Done():
			w.stop()
			return
		case cmd := <-w.cmdCh:
			w.handleCmd(cmd)
		case <-reconnectTicker.C:
			if w.mode == WorkerModeDisconnected && !w.connector.Ok() {
				if err := w.tryConnect(); err != nil {
					log.Error("worker: connect failed! ", slog.String("name", w.name), log.ErrWrap(err))
				} else {
					log.Info("worker: connect success! ", slog.String("name", w.name))
				}
			}
		case <-heartbeatTicker.C:
			if w.mode == WorkerModeHeartbeat && w.connector.Ok() {
				if err := w.connector.Heartbeat(); err != nil {
					log.Error("worker: heartbeat failed! ", slog.String("name", w.name), log.ErrWrap(err))
					w.closeConnWithEvent()
				} else {
					log.Info("worker: heartbeat success! ", slog.String("name", w.name))
				}
			}
		}
	}

}

func (w *worker) SendCmd(cmd WorkerCmd) {
	select {
	case w.cmdCh <- cmd:
	default:
		log.Error("worker: send cmd failed! ", slog.String("name", w.name))
	}
}

func (w *worker) handleCmd(cmd WorkerCmd) {
	switch cmd.typ {
	case CmdStartHeartbeat:
		if w.connector.Ok() && w.mode == WorkerModeConnectedIdle {
			w.mode = WorkerModeHeartbeat
		}
	case CmdStopHeartbeat:
		if w.connector.Ok() && w.mode == WorkerModeHeartbeat {
			w.mode = WorkerModeConnectedIdle
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
	if w.mode != WorkerModeDisconnected {
		w.eventer.SendEvent(Event{typ: w.evts.disconnected})
	}
	w.mode = WorkerModeDisconnected
}

func (w *worker) closeConnSilently() {
	if w.connector.Ok() {
		w.connector.Close()
	}
	w.mode = WorkerModeDisconnected
}

func (w *worker) tryConnect() error {
	if err := w.connector.Connect(time.Second * 5); err != nil {
		return err
	}
	w.mode = WorkerModeConnectedIdle
	w.eventer.SendEvent(Event{typ: w.evts.connected})
	return nil
}

func (w *worker) detached() {
	if !w.connector.Ok() {
		return
	}

	cc := w.connector.Detached()
	w.mode = WorkerModeDetached
	w.eventer.SendEvent(Event{typ: w.evts.detached, rwc: cc})
}

package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/internal/conn"
	"github.com/yzhlove/peids/app/log"
)

type worker struct {
	name      string
	ctx       context.Context
	cfg       *config.Config
	eventer   Eventer
	cmdCh     chan WorkerCmd
	mode      WorkerMode
	connector conn.Connector
}

func newWorker(ctx context.Context, name string, cfg *config.Config, e Eventer, connector conn.Connector) Worker {
	return &worker{
		name:      name,
		ctx:       ctx,
		cfg:       cfg,
		eventer:   e,
		connector: connector,
		cmdCh:     make(chan WorkerCmd, 16),
	}
}

func (w *worker) Stop() {
	if w.connector != nil {
		w.connector.Close()
		w.connector = nil
	}
	w.mode = WorkerModeDisconnected
}

func (w *worker) Run() {

	reconnectTicker := time.NewTicker(time.Second * 45)
	defer reconnectTicker.Stop()

	heartbeatTicker := time.NewTicker(time.Second * 15)
	defer heartbeatTicker.Stop()

	// 首先尝试连接一次
	if err := w.tryConnect(); err != nil {
		log.Error("worker: first connect redis failed! ", slog.String("name", w.name), log.ErrWrap(err))
	}

	for {
		select {
		case <-w.ctx.Done():
			w.Stop()
			return
		case cmd := <-w.cmdCh:
			w.handleCmd(cmd)
		case <-reconnectTicker.C:
			if w.mode == WorkerModeDisconnected && !w.connector.Ok() {
				if err := w.tryConnect(); err != nil {
					log.Error("worker: connect redis failed! ", slog.String("name", w.name), log.ErrWrap(err))
				} else {
					log.Info("worker: connect redis success! ", slog.String("name", w.name))
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
		w.eventer.SendEvent(Event{typ: EvRedisDisconnected})
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
	w.eventer.SendEvent(Event{typ: EvRedisConnected})
	return nil
}

func (w *worker) detached() {
	if !w.connector.Ok() {
		return
	}

	cc := w.connector.Detached()
	w.mode = WorkerModeDetached
	w.eventer.SendEvent(Event{typ: EvRedisConnectDetached, rwc: cc})
}

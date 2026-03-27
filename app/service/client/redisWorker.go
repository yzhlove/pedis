package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/internal/medis"
	"github.com/yzhlove/peids/app/log"
)

type redisWorker struct {
	name    string
	ctx     context.Context
	cfg     *config.Config
	eventer Eventer
	cmdCh   chan WorkerCmd
	mode    WorkerMode
	medis   medis.RedisManager
}

func NewRedisWork(ctx context.Context, cfg *config.Config, e Eventer) Worker {
	return &redisWorker{
		name:    "redis-worker",
		ctx:     ctx,
		cfg:     cfg,
		eventer: e,
		cmdCh:   make(chan WorkerCmd, 16),
	}
}

func (w *redisWorker) Stop() {
	if w.medis != nil {
		w.medis.Close()
		w.medis = nil
	}
	w.mode = WorkerModeDisconnected
}

func (w *redisWorker) Run() {

	reconnectTicket := time.NewTicker(time.Minute)
	defer reconnectTicket.Stop()

	heartbeatTicket := time.NewTicker(time.Second * 30)
	defer heartbeatTicket.Stop()

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
			w.handelCmd(cmd)
		case <-reconnectTicket.C:
			if w.mode == WorkerModeDisconnected && w.medis == nil {
				if err := w.tryConnect(); err != nil {
					log.Error("worker: connect redis failed! ", slog.String("name", w.name), log.ErrWrap(err))
				} else {
					log.Info("worker: connect redis success! ", slog.String("name", w.name))
				}
			}
		case <-heartbeatTicket.C:
			if w.mode == WorkerModeHeartbeat && w.medis != nil {
				if err := w.medis.Heartbeat(); err != nil {
					log.Error("worker: heartbeat failed! ", slog.String("name", w.name), log.ErrWrap(err))
					w.closeConnWithEvent()
				} else {
					log.Info("worker: heartbeat success! ", slog.String("name", w.name))
				}
			}
		}
	}

}

func (w *redisWorker) SendCmd(cmd WorkerCmd) {
	select {
	case w.cmdCh <- cmd:
	default:
		log.Error("worker: send cmd failed! ", slog.String("name", w.name))
	}
}

func (w *redisWorker) handelCmd(cmd WorkerCmd) {
	switch cmd.typ {
	case CmdStartHeartbeat:
		if w.medis != nil && w.mode == WorkerModeConnectedIdle {
			w.mode = WorkerModeHeartbeat
		}
	case CmdStopHeartbeat:
		if w.medis != nil && w.mode == WorkerModeHeartbeat {
			w.mode = WorkerModeConnectedIdle
		}
	case CmdDetachForBridge:
		w.detached()
	case CmdShutdown:
		w.closeConnSilently()
	}
}

func (w *redisWorker) closeConnWithEvent() {
	if w.medis != nil {
		w.medis.Close()
		w.medis = nil
	}
	if w.mode != WorkerModeDisconnected {
		w.eventer.SendEvent(Event{typ: EvRedisDisconnected})
	}
	w.mode = WorkerModeDisconnected
}

func (w *redisWorker) closeConnSilently() {
	if w.medis != nil {
		w.medis.Close()
		w.medis = nil
	}
	w.mode = WorkerModeDisconnected
}

func (w *redisWorker) tryConnect() error {
	m := medis.New(w.cfg.CliRedisHost, w.cfg.CliRedisPort)
	if err := m.Connect(time.Second * 5); err != nil {
		return err
	}
	w.medis = m
	w.mode = WorkerModeConnectedIdle
	w.eventer.SendEvent(Event{typ: EvRedisConnected})
	return nil
}

func (w *redisWorker) detached() {
	if w.medis == nil {
		return
	}

	conn := w.medis.TcpConn()
	w.medis = nil
	w.mode = WorkerModeDetached
	w.eventer.SendEvent(Event{typ: EvRedisConnectDetached, rwc: conn})
}

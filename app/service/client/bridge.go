package client

import (
	"io"
	"sync"

	"github.com/yzhlove/peids/app/log"
)

type bridgeController struct {
	sync.Mutex
	Eventer
	running   bool
	unixConn  io.ReadWriteCloser
	redisConn io.ReadWriteCloser
}

func NewBridgeController(e Eventer) *bridgeController {
	return &bridgeController{
		Eventer: e,
	}
}

func (b *bridgeController) Start(u, r io.ReadWriteCloser) {
	b.Lock()
	if b.running {
		b.Unlock()
		return
	}

	b.running = true
	b.unixConn = u
	b.redisConn = r
	b.Unlock()
	go b.transport()
	log.Info("bridge: start!")
}

func (b *bridgeController) transport() {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(b.unixConn, b.redisConn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(b.redisConn, b.unixConn)
		errCh <- err
	}()
	err := <-errCh
	b.Stop()
	if !b.SendEvent(Event{typ: EvBridgeStopped, err: err}) {
		log.Error("bridge: write event failed! ", log.ErrWrap(err))
	}
}

func (b *bridgeController) Stop() {
	b.Lock()
	defer b.Unlock()

	if !b.running {
		return
	}

	if b.unixConn != nil {
		b.unixConn.Close()
	}
	if b.redisConn != nil {
		b.redisConn.Close()
	}
	b.running = false
	b.unixConn = nil
	b.redisConn = nil
	log.Info("bridge: stop!")
}

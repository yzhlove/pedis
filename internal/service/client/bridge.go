package client

import (
	"net"
	"sync"

	"github.com/yzhlove/pedis/internal/helper"
	"github.com/yzhlove/pedis/internal/log"
)

type bridgeController struct {
	sync.Mutex
	Eventer
	running   bool
	unixConn  net.Conn
	redisConn net.Conn
}

func newBridgeController(e Eventer) *bridgeController {
	return &bridgeController{Eventer: e}
}

func (b *bridgeController) Start(u, r net.Conn) {
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
	log.Info("bridge: starting")
}

func (b *bridgeController) transport() {
	if err := helper.Bridge(b.unixConn, b.redisConn); err != nil {
		log.Error("bridge: transport error", log.ErrWrap(err))
	}
	b.Stop()
	b.SendEvent(Event{typ: BridgeStopped})
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
	log.Info("bridge: stopped")
}

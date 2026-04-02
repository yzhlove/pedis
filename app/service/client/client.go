package client

import (
	"context"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/log"
	"github.com/yzhlove/peids/app/service"
)

type client struct {
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc
	mgr    *manager
}

func New(cfg *config.Config) service.Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &client{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *client) Init() error {
	if c.isRunning() {
		c.mgr = newManager(c.ctx, c.cfg)
	}
	return nil
}

func (c *client) Start() error {
	if c.isRunning() {
		log.Info("service: client is starting! ")
		c.mgr.Run()
	}
	return nil
}

func (c *client) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.isRunning() {
		c.mgr.stop()
	}
	return nil
}

func (c *client) isRunning() bool {
	return c.cfg.Role == config.ClientRole
}

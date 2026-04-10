package client

import (
	"context"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/service"
)

type clientService struct {
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc
	mgr    *manager
}

// New creates a client Service for the given config.
func New(cfg *config.Config) service.Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &clientService{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *clientService) Init() error {
	if c.isRunning() {
		c.mgr = newManager(c.ctx, c.cfg)
	}
	return nil
}

func (c *clientService) Start() error {
	if c.isRunning() {
		log.Info("service: client is starting")
		c.mgr.Run()
	}
	return nil
}

func (c *clientService) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func (c *clientService) isRunning() bool {
	return c.cfg.Role == config.ClientRole
}

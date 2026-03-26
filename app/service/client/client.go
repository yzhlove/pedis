package client

import (
	"context"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/service"
)

type client struct {
	cfg    *config.Config
	ctx    context.Context
	cancel context.CancelFunc
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

	return nil
}

func (c *client) Start() error {

	return nil
}

func (c *client) Stop() error {

	return nil
}

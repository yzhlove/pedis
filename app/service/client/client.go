package client

import (
	"context"
	"sync/atomic"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/internal/medis"
	"github.com/yzhlove/peids/app/service"
)

type client struct {
	cfg          *config.Config
	ctx          context.Context
	cancel       context.CancelFunc
	redis        medis.RedisManager
	redisStatus  atomic.Bool
	serviceReady atomic.Bool
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

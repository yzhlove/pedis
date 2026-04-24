package server

import (
	"context"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/service"
)

type serverService struct {
	cfg      *config.Config
	ctx      context.Context
	cancel   context.CancelFunc
	registry *Registry
	unix     *unixServer
	redis    *redisServer
}

// New creates a server Service for the given config.
func New(cfg *config.Config) service.Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &serverService{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *serverService) isRunning() bool {
	return s.cfg.Role == config.ServerRole
}

func (s *serverService) Init() error {
	if !s.isRunning() {
		return nil
	}
	s.registry = newRegistry()
	s.unix = newUnixServer(s.ctx, s.cfg, s.registry)
	s.redis = newRedisServer(s.ctx, s.cfg, s.registry)
	return nil
}

func (s *serverService) Start() error {
	if !s.isRunning() {
		return nil
	}
	log.Info("server-service: server is starting")

	errCh := make(chan error, 2)
	go func() { errCh <- s.unix.serve() }()
	go func() { errCh <- s.redis.serve() }()

	select {
	case <-s.ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *serverService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.unix != nil {
		s.unix.close()
	}
	if s.redis != nil {
		s.redis.close()
	}
	if s.registry != nil {
		s.registry.Close()
	}
	return nil
}

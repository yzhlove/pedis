package server

import (
	"context"
	"log/slog"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"go.uber.org/fx"
)

// New constructs the unix/redis listeners (when configured as a server) and
// registers their lifecycle on the fx App. Serve errors trigger an app-wide
// shutdown via fx.Shutdowner so siblings stop cleanly.
func New(lc fx.Lifecycle, sh fx.Shutdowner, cfg *config.Config) error {
	if cfg.Role != config.ServerRole {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	registry := newRegistry()
	unix := newUnixServer(ctx, cfg, registry)
	redis := newRedisServer(ctx, cfg, registry)

	serve := func(name string, fn func() error) {
		if err := fn(); err != nil {
			log.Error("server-service: serve failed",
				log.ErrWrap(err), slog.String("listener", name))
			_ = sh.Shutdown(fx.ExitCode(1))
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("server-service: server is starting")
			go serve("unix", unix.serve)
			go serve("redis", redis.serve)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			unix.close()
			redis.close()
			registry.Close()
			return nil
		},
	})
	return nil
}

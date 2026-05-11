package client

import (
	"context"
	"errors"
	"sync"

	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"go.uber.org/fx"
)

var errClientNamaEmpty = errors.New("client-service: client name cannot be empty")

// New constructs the client-side managers (when configured as a client) and
// registers their lifecycle on the fx App. Returns nil for non-client roles
// so the constructor is safe to invoke unconditionally.
func New(lc fx.Lifecycle, cfg *config.Config) error {
	if cfg.Role != config.ClientRole {
		return nil
	}
	if len(cfg.CliName) == 0 {
		return errClientNamaEmpty
	}

	ctx, cancel := context.WithCancel(context.Background())
	mgrs := make([]*manager, 0, config.MaxClientConns)
	for range config.MaxClientConns {
		mgrs = append(mgrs, newManager(ctx, cfg))
	}

	var wg sync.WaitGroup

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("client-service: client is starting")
			for _, m := range mgrs {
				wg.Add(1)
				go func(m *manager) {
					defer wg.Done()
					m.Run()
				}(m)
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
	return nil
}

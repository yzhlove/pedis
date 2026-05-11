package main

import (
	"fmt"
	"log/slog"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/module"
	"github.com/yzhlove/pedis/internal/service/client"
	"github.com/yzhlove/pedis/internal/service/server"
	"github.com/yzhlove/pedis/internal/text"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

var (
	buildDate  string
	goVersion  string
	appVersion string
)

func main() {
	app := fx.New(
		fx.Provide(
			config.New,
			fx.Annotate(cipher.New, fx.ResultTags(`group:"modules"`)),
			fx.Annotate(text.New, fx.ResultTags(`group:"modules"`)),
		),
		fx.WithLogger(func(cfg *config.Config) fxevent.Logger {
			log.Init(cfg, slog.Attr{Key: "app", Value: slog.StringValue("pedis")})
			if cfg.ENV == "production" {
				return fxevent.NopLogger
			}
			l := &fxevent.SlogLogger{Logger: log.Get()}
			l.UseErrorLevel(slog.LevelError)
			return l
		}),
		fx.Invoke(func() {
			log.Info("app start",
				slog.String("build_date", buildDate),
				slog.String("go_version", goVersion),
				slog.String("app_version", appVersion))
		}),
		fx.Invoke(func(in struct {
			fx.In
			Modules []module.Module `group:"modules"`
		}) error {
			if err := module.Apply(in.Modules...); err != nil {
				return fmt.Errorf("module: apply error: %v", err)
			}
			return nil
		}),
		fx.Invoke(client.New, server.New),
	)

	if err := app.Err(); err != nil {
		log.Error("app start failed", log.ErrWrap(err))
		return
	}
	app.Run()
}

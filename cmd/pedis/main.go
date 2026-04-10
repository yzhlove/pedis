package main

import (
	"log/slog"

	"github.com/yzhlove/pedis/internal/cipher"
	"github.com/yzhlove/pedis/internal/config"
	"github.com/yzhlove/pedis/internal/log"
	"github.com/yzhlove/pedis/internal/module"
	"github.com/yzhlove/pedis/internal/service"
	"github.com/yzhlove/pedis/internal/service/client"
	"github.com/yzhlove/pedis/internal/text"
	"go.uber.org/dig"
)

func main() {
	type in struct {
		dig.In
		Config   *config.Config
		Services []service.Service `group:"services"`
		Modules  []module.Module   `group:"modules"`
	}

	container := dig.New()
	container.Provide(config.New)
	container.Provide(client.New, dig.Group("services"))
	container.Provide(text.New, dig.Group("modules"))
	container.Provide(cipher.New, dig.Group("modules"))

	if err := container.Invoke(func(i in) error {
		log.Init(i.Config, slog.Attr{Key: "app", Value: slog.StringValue("pedis")})
		if err := module.Apply(i.Modules...); err != nil {
			return err
		}
		return service.Run(i.Services...)
	}); err != nil {
		log.Error("app start failed", log.ErrWrap(err))
	}
}

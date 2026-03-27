package main

import (
	"log/slog"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/log"
	"github.com/yzhlove/peids/app/service"
	"github.com/yzhlove/peids/app/service/client"
	"go.uber.org/dig"
)

func main() {

	type in struct {
		dig.In
		Config   *config.Config
		Services []service.Service `group:"services"`
	}

	container := dig.New()
	container.Provide(config.New)
	container.Provide(client.New, dig.Group("services"))

	if err := container.Invoke(func(i in) error {
		log.Init(i.Config, slog.Attr{Key: "name", Value: slog.StringValue("pedis")})
		return service.Run(i.Services...)
	}); err != nil {
		log.Error("app start failed! ", log.ErrWrap(err))
	}
}

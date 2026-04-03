package main

import (
	"log/slog"

	"github.com/yzhlove/peids/app/config"
	"github.com/yzhlove/peids/app/log"
	"github.com/yzhlove/peids/app/modules"
	"github.com/yzhlove/peids/app/modules/text"
	"github.com/yzhlove/peids/app/service"
	"github.com/yzhlove/peids/app/service/client"
	"go.uber.org/dig"
)

func main() {

	type in struct {
		dig.In
		Config   *config.Config
		Services []service.Service `group:"services"`
		Modules  []modules.Modules `group:"modules"`
	}

	container := dig.New()
	container.Provide(config.New)
	container.Provide(client.New, dig.Group("services"))
	container.Provide(text.New, dig.Group("modules"))

	if err := container.Invoke(func(i in) error {
		log.Init(i.Config, slog.Attr{Key: "app", Value: slog.StringValue("pedis")})
		if err := modules.Apply(i.Modules...); err != nil {
			return err
		}
		return service.Run(i.Services...)
	}); err != nil {
		log.Error("app start failed! ", log.ErrWrap(err))
	}
}

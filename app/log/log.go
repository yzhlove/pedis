package log

import (
	"log/slog"
	"os"
	"time"

	"github.com/yzhlove/peids/app/config"
)

func initLog(h slog.Handler) {

}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.Format(time.RFC3339))
		}
	case slog.SourceKey:

	}
	return a
}

func init() {

}

func Init(cfg *config.Config, attrs ...slog.Attr) {
	var h slog.Handler
	switch cfg.ENV {
	case "production":
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource:   true,
			Level:       slog.LevelError,
			ReplaceAttr: replaceAttr,
		})
	default:
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			AddSource:   true,
			Level:       slog.LevelDebug,
			ReplaceAttr: replaceAttr,
		})
	}
	initLog(h)
}

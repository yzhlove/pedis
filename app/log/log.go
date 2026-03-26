package log

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yzhlove/peids/app/config"
)

func initLog(h slog.Handler) {
	if h == nil {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			AddSource:   true,
			Level:       slog.LevelDebug,
			ReplaceAttr: replaceAttr,
		})
	}
	_log = slog.New(h)
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.Format(time.RFC3339))
		}
	case slog.SourceKey:
		src, ok := a.Value.Any().(*slog.Source)
		if !ok || src == nil {
			return a
		}
		if i := strings.LastIndexByte(src.File, '/'); i != -1 {
			return slog.String(a.Key, src.File[i+1:]+":"+strconv.Itoa(src.Line))
		}
	}
	return a
}

var _log *slog.Logger

func init() {
	initLog(nil)
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
	if len(attrs) > 0 {
		anyAttrs := make([]any, len(attrs))
		for i := range attrs {
			anyAttrs[i] = attrs[i]
		}
		_log = _log.With(anyAttrs...)
	}
}

func Info(msg string, args ...any) {
	_log.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	_log.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	_log.Error(msg, args...)
}

func Debug(msg string, args ...any) {
	_log.Debug(msg, args...)
}

func ErrWrap(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "<nil>")
	}
	return slog.Attr{Key: "error", Value: slog.StringValue(err.Error())}
}

package log

import (
	"context"
	"log/slog"
	"os"
	"runtime"
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

var bg = context.Background()

func output(level slog.Level, msg string, attrs ...slog.Attr) {
	if !_log.Enabled(bg, level) {
		return
	}
	var pcs [1]uintptr
	// skip: runtime.Callers, output, public wrapper (Info/Warn/Error/Debug)
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.AddAttrs(attrs...)
	_ = _log.Handler().Handle(bg, r)
}

func Info(msg string, attrs ...slog.Attr) {
	output(slog.LevelInfo, msg, attrs...)
}

func Warn(msg string, attrs ...slog.Attr) {
	output(slog.LevelWarn, msg, attrs...)
}

func Error(msg string, attrs ...slog.Attr) {
	output(slog.LevelError, msg, attrs...)
}

func Debug(msg string, attrs ...slog.Attr) {
	output(slog.LevelDebug, msg, attrs...)
}

func ErrWrap(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "<nil>")
	}
	return slog.Attr{Key: "error", Value: slog.StringValue(err.Error())}
}

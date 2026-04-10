package log

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/yzhlove/pedis/internal/config"
)

func TestLog(t *testing.T) {
	Info("hello world!", slog.String("key", "value"))
}

func TestErrWrapNil(t *testing.T) {
	attr := ErrWrap(nil)
	if attr.Key != "error" {
		t.Fatalf("unexpected key: %q", attr.Key)
	}
	if got := attr.Value.String(); got != "<nil>" {
		t.Fatalf("unexpected value: %q", got)
	}
}

func TestErrWrapError(t *testing.T) {
	attr := ErrWrap(errors.New("boom"))
	if attr.Key != "error" {
		t.Fatalf("unexpected key: %q", attr.Key)
	}
	if got := attr.Value.String(); got != "boom" {
		t.Fatalf("unexpected value: %q", got)
	}
}

func TestReplaceAttrSourceTypeMismatch(t *testing.T) {
	attr := slog.Any(slog.SourceKey, "not-a-source")
	got := replaceAttr(nil, attr)
	if got.Key != slog.SourceKey {
		t.Fatalf("unexpected key: %q", got.Key)
	}
	if got.Value.Kind() != slog.KindString || got.Value.String() != "not-a-source" {
		t.Fatalf("unexpected value after replace: %#v", got.Value)
	}
}

func TestInitNilConfigAndAttrs(t *testing.T) {
	Init(&config.Config{}, slog.String("service", "pedis-test"))
	Info("message from empty-config init")

	Init(&config.Config{ENV: "dev"}, slog.String("service", "pedis-dev"))
	Debug("message from dev init")
}

// initDiscardLogger 将日志输出重定向到 io.Discard，排除 I/O 干扰。
func initDiscardLogger() {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	initLog(h)
}

func BenchmarkInfo(b *testing.B) {
	initDiscardLogger()
	for b.Loop() {
		Info("benchmark message")
	}
}

func BenchmarkInfoWithAttrs(b *testing.B) {
	initDiscardLogger()
	for b.Loop() {
		Info("benchmark message",
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		)
	}
}

func BenchmarkError(b *testing.B) {
	initDiscardLogger()
	err := errors.New("something went wrong")
	for b.Loop() {
		Error("benchmark error", ErrWrap(err))
	}
}

func BenchmarkDebugDisabled(b *testing.B) {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelError,
	})
	initLog(h)
	for b.Loop() {
		Debug("this should be filtered out")
	}
}

func BenchmarkDirectSlog(b *testing.B) {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	l := slog.New(h)
	for b.Loop() {
		l.Info("benchmark message")
	}
}

func BenchmarkErrWrap(b *testing.B) {
	err := errors.New("benchmark error")
	for b.Loop() {
		_ = ErrWrap(err)
	}
}

package log

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/yzhlove/peids/app/config"
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

// BenchmarkInfo 测试最常用的 Info 调用基线性能。
func BenchmarkInfo(b *testing.B) {
	initDiscardLogger()
	for b.Loop() {
		Info("benchmark message")
	}
}

// BenchmarkInfoWithAttrs 测试携带多个 slog.Attr 参数时的性能。
func BenchmarkInfoWithAttrs(b *testing.B) {
	initDiscardLogger()
	for b.Loop() {
		Info("benchmark message",
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		)
	}
}

// BenchmarkError 测试 Error 级别调用性能。
func BenchmarkError(b *testing.B) {
	initDiscardLogger()
	err := errors.New("something went wrong")
	for b.Loop() {
		Error("benchmark error", ErrWrap(err))
	}
}

// BenchmarkDebugDisabled 测试日志级别被过滤时的 fast-path 性能。
// 将 handler 设为 Error 级别，Debug 调用应被 Enabled() 快速跳过。
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

// BenchmarkDirectSlog 作为基准对照：直接调用 slog.Logger，无包装层。
// 对比此结果与 BenchmarkInfo，可以量化包装层带来的额外开销。
func BenchmarkDirectSlog(b *testing.B) {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	logger := slog.New(h)
	for b.Loop() {
		logger.Info("benchmark message")
	}
}

// BenchmarkErrWrap 单独测试 ErrWrap 的 Attr 构造开销。
func BenchmarkErrWrap(b *testing.B) {
	err := errors.New("benchmark error")
	for b.Loop() {
		_ = ErrWrap(err)
	}
}

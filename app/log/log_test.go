package log

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/yzhlove/peids/app/config"
)

func TestLog(t *testing.T) {
	Info("hello world!")
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
	Init(nil, slog.String("service", "pedis-test"))
	Info("message from nil-config init")

	Init(&config.Config{ENV: "dev"}, slog.String("service", "pedis-dev"))
	Debug("message from dev init")
}

package cfglogger

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestFormatMessageRendersSlogAttrs(t *testing.T) {
	err := errors.New("read /tmp/x: is a directory")

	got := FormatMessage("cfggo: Init failed",
		"config", "Config",
		"err", err,
		slog.String("source", "file source"),
	)
	want := `cfggo: Init failed config=Config err="read /tmp/x: is a directory" source="file source"`
	if got != want {
		t.Fatalf("FormatMessage() = %q, want %q", got, want)
	}
}

func TestFormatMessageMarksMalformedAttrs(t *testing.T) {
	got := FormatMessage("msg", 123, "lonely")
	want := "msg !BADKEY=123 !MISSING=lonely"
	if got != want {
		t.Fatalf("FormatMessage() = %q, want %q", got, want)
	}
}

func TestPlainLoggerPassesFormattedMessageWithoutArgs(t *testing.T) {
	var logs bytes.Buffer
	logger := Plain(NewPrintfLogger(nil, nil, nil, func(format string, args ...any) {
		fmt.Fprintf(&logs, format, args...)
	}))

	logger = logger.(WithAttrer).With("config", "Config")
	logger.Error("cfggo: Init failed", "err", errors.New("boom"))

	got := logs.String()
	want := "cfggo: Init failed config=Config err=boom"
	if got != want {
		t.Fatalf("plain logger output = %q, want %q", got, want)
	}
}

func TestPrintfLoggerLevelsAndWithAttrs(t *testing.T) {
	var logs bytes.Buffer
	write := func(prefix string) PrintfFunc {
		return func(format string, args ...any) {
			fmt.Fprintf(&logs, prefix+format+"\n", args...)
		}
	}

	logger := NewPrintfLogger(write("debug:"), write("info:"), write("warn:"), nil)
	logger.Debug("starting", "port", 8080)
	logger.Info("ready", slog.String("addr", "127.0.0.1:8080"))
	logger.Warn("slow", "duration", "2s")
	logger.Error("discarded")

	child := logger.(WithAttrer).With("config", "api")
	child.Info("child", "ok", true)

	got := logs.String()
	for _, want := range []string{
		"debug:starting port=8080",
		"info:ready addr=127.0.0.1:8080",
		"warn:slow duration=2s",
		"info:child config=api ok=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("logs = %q, want substring %q", got, want)
		}
	}
	if strings.Contains(got, "discarded") {
		t.Fatalf("nil error logger should discard output, got %q", got)
	}
}

func TestPlainNilAndIdempotent(t *testing.T) {
	if _, ok := Plain(nil).(*NoopLogger); !ok {
		t.Fatal("Plain(nil) should return NoopLogger")
	}

	logger := Plain(NewPrintfLogger(nil, nil, nil, nil))
	if Plain(logger) != logger {
		t.Fatal("Plain() should return existing plain logger unchanged")
	}
}

func TestDefaultLoggerLevelAndWith(t *testing.T) {
	var logs bytes.Buffer
	logger := NewDefaultLoggerWithWriter(&logs)

	if got := logger.Level(); got != slog.LevelInfo {
		t.Fatalf("initial Level() = %v, want info", got)
	}
	logger.Debug("hidden")
	if strings.Contains(logs.String(), "hidden") {
		t.Fatalf("debug log emitted before lowering level: %q", logs.String())
	}

	logger.SetLevel(slog.LevelDebug)
	if got := logger.Level(); got != slog.LevelDebug {
		t.Fatalf("Level() = %v, want debug", got)
	}
	logger.Debug("visible", "answer", 42)
	logger.With("config", "api").Info("ready")

	got := logs.String()
	for _, want := range []string{"visible", "answer=42", "config=api", "ready"} {
		if !strings.Contains(got, want) {
			t.Fatalf("logs = %q, want substring %q", got, want)
		}
	}
}

func TestNoopLoggerWithReturnsSelf(t *testing.T) {
	logger := &NoopLogger{}
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
	if logger.With("config", "api") != logger {
		t.Fatal("NoopLogger.With() should return the same logger")
	}
}

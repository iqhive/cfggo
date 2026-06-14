package cfglogger

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
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

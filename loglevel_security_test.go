package cfggo

import (
	"strconv"
	"testing"
)

func TestSetLogLevelDoesNotOverflowIntoDifferentLevel(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("the overflowing value is not representable as LogLevel on 32-bit systems")
	}
	previous := LogLevel(currentLevel.Load())
	t.Cleanup(func() { SetLogLevel(previous) })

	overflowingNone := LogLevel(int64(1)<<32 + int64(LogLevelNone))
	SetLogLevel(overflowingNone)
	if got := LogLevel(currentLevel.Load()); got != LogLevelInfo {
		t.Fatalf("unknown overflowing level stored as %v, want the same Info fallback used by slogLevel", got)
	}
}

package cfglogger

import (
	"io"
	"log/slog"
	"os"
)

// Logger is the minimal logging surface cfggo depends on.
//
// The method set is deliberately identical to the leveled methods of
// *slog.Logger, so a *slog.Logger satisfies this interface directly with no
// adapter:
//
//	cfg.Init(cfg, cfggo.WithLogger(slog.Default()))
//
// Following slog conventions, args are treated as alternating key/value
// attribute pairs. cfggo's own internal log calls pass a single pre-formatted
// message and no args, so any Logger that simply prints msg behaves correctly.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// DefaultLogger is the default Logger implementation. It is backed by the
// standard library's log/slog and, crucially, honours a configurable level: a
// *slog.LevelVar drives the underlying handler so that raising the verbosity
// (e.g. via cfggo.SetLogLevel(cfggo.LogLevelDebug)) actually surfaces Debug
// output instead of being dropped by slog's default Info threshold.
type DefaultLogger struct {
	level  *slog.LevelVar
	logger *slog.Logger
}

// NewDefaultLogger creates a DefaultLogger that writes to os.Stderr.
func NewDefaultLogger() *DefaultLogger {
	return NewDefaultLoggerWithWriter(os.Stderr)
}

// NewDefaultLoggerWithWriter creates a DefaultLogger that writes to w.
func NewDefaultLoggerWithWriter(w io.Writer) *DefaultLogger {
	level := new(slog.LevelVar) // zero value == slog.LevelInfo
	return &DefaultLogger{
		level:  level,
		logger: slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})),
	}
}

// SetLevel adjusts the minimum level this logger emits. It is safe to call
// concurrently with logging because it is backed by a *slog.LevelVar.
func (dl *DefaultLogger) SetLevel(level slog.Level) {
	dl.level.Set(level)
}

// Level returns the current minimum level this logger emits.
func (dl *DefaultLogger) Level() slog.Level {
	return dl.level.Level()
}

func (dl *DefaultLogger) Debug(msg string, args ...any) { dl.logger.Debug(msg, args...) }
func (dl *DefaultLogger) Info(msg string, args ...any)  { dl.logger.Info(msg, args...) }
func (dl *DefaultLogger) Warn(msg string, args ...any)  { dl.logger.Warn(msg, args...) }
func (dl *DefaultLogger) Error(msg string, args ...any) { dl.logger.Error(msg, args...) }

// NoopLogger is a Logger that discards everything. Useful for silencing cfggo
// entirely on a per-instance basis via WithLogger(&cfglogger.NoopLogger{}).
type NoopLogger struct{}

func (nl *NoopLogger) Debug(msg string, args ...any) {}
func (nl *NoopLogger) Info(msg string, args ...any)  {}
func (nl *NoopLogger) Warn(msg string, args ...any)  {}
func (nl *NoopLogger) Error(msg string, args ...any) {}

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

// LevelSetter is an optional interface a Logger may implement to participate in
// cfggo.SetLogLevel. When a Logger implements it, SetLogLevel forwards the
// requested level (mapped onto slog.Level) so custom loggers can opt in to
// cfggo's level control instead of SetLogLevel silently doing nothing
//
// The built-in DefaultLogger implements this interface
type LevelSetter interface {
	SetLevel(level slog.Level)
}

// WithAttrer is an optional interface a Logger may implement to return a child
// logger with the given slog-style key/value attributes permanently attached.
//
// When a Logger implements it, cfggo binds the configuration's name onto the
// instance logger once during Init (as a "config" attribute), so every line a
// Structure emits is automatically attributable to that instance without each
// call site repeating the name. DefaultLogger implements this; a raw
// *slog.Logger is also handled because its own With returns *slog.Logger (which
// already satisfies Logger)
type WithAttrer interface {
	With(args ...any) Logger
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

// With returns a child DefaultLogger that prepends args to every record. The
// returned logger shares the parent's *slog.LevelVar,
// so SetLogLevel continues to control its verbosity
func (dl *DefaultLogger) With(args ...any) Logger {
	return &DefaultLogger{level: dl.level, logger: dl.logger.With(args...)}
}

// NoopLogger is a Logger that discards everything. Useful for silencing cfggo
// entirely on a per-instance basis via WithLogger(&cfglogger.NoopLogger{}).
type NoopLogger struct{}

func (nl *NoopLogger) Debug(msg string, args ...any) {}
func (nl *NoopLogger) Info(msg string, args ...any)  {}
func (nl *NoopLogger) Warn(msg string, args ...any)  {}
func (nl *NoopLogger) Error(msg string, args ...any) {}

// With returns the same NoopLogger: attaching attributes to
// a logger that discards everything is still a no-op
func (nl *NoopLogger) With(args ...any) Logger { return nl }

package cfglogger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Logger is the minimal logging surface cfggo depends on.
//
// The method set is deliberately identical to the leveled methods of
// *slog.Logger, so a *slog.Logger satisfies this interface directly with no
// adapter:
//
//	cfg.Init(cfg, cfggo.WithLogger(slog.Default()))
//
// Following slog conventions, args are treated as attributes: either alternating
// key/value pairs or slog.Attr values. Loggers that forward msg and args to a
// printf-style API should wrap themselves with Plain first so attributes are
// rendered into msg instead of being treated as printf arguments.
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

// FormatMessage renders a slog-style log message and attributes as a single
// human-readable line. It is intended for adapters around printf/plain loggers;
// structured loggers should keep receiving msg and args separately.
func FormatMessage(msg string, args ...any) string {
	if len(args) == 0 {
		return msg
	}

	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i < len(args); {
		if attr, ok := args[i].(slog.Attr); ok {
			appendAttr(&b, attr)
			i++
			continue
		}

		key, ok := args[i].(string)
		if !ok {
			appendKeyValue(&b, "!BADKEY", args[i])
			i++
			continue
		}
		if i+1 >= len(args) {
			appendKeyValue(&b, "!MISSING", key)
			i++
			continue
		}
		appendKeyValue(&b, key, args[i+1])
		i += 2
	}
	return b.String()
}

func appendAttr(b *strings.Builder, attr slog.Attr) {
	if attr.Key == "" {
		return
	}
	appendKeyValue(b, attr.Key, attr.Value)
}

func appendKeyValue(b *strings.Builder, key string, value any) {
	b.WriteByte(' ')
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(formatValue(value))
}

func formatValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	if slogValue, ok := value.(slog.Value); ok {
		slogValue = slogValue.Resolve()
		if slogValue.Kind() == slog.KindAny {
			value = slogValue.Any()
		} else {
			value = slogValue.String()
		}
	}

	text := fmt.Sprint(value)
	if text == "" || strings.ContainsAny(text, " \t\r\n\"=") {
		return strconv.Quote(text)
	}
	return text
}

// Plain wraps a Logger so it receives a single formatted message and no attrs.
// Use it for custom loggers backed by fmt.Printf/log.Printf-style APIs.
func Plain(logger Logger) Logger {
	if logger == nil {
		return &NoopLogger{}
	}
	if _, ok := logger.(*plainLogger); ok {
		return logger
	}
	return &plainLogger{logger: logger}
}

type plainLogger struct {
	logger Logger
	attrs  []any
}

func (pl *plainLogger) Debug(msg string, args ...any) {
	pl.logger.Debug(FormatMessage(msg, mergeAttrs(pl.attrs, args)...))
}
func (pl *plainLogger) Info(msg string, args ...any) {
	pl.logger.Info(FormatMessage(msg, mergeAttrs(pl.attrs, args)...))
}
func (pl *plainLogger) Warn(msg string, args ...any) {
	pl.logger.Warn(FormatMessage(msg, mergeAttrs(pl.attrs, args)...))
}
func (pl *plainLogger) Error(msg string, args ...any) {
	pl.logger.Error(FormatMessage(msg, mergeAttrs(pl.attrs, args)...))
}

func (pl *plainLogger) With(args ...any) Logger {
	return &plainLogger{logger: pl.logger, attrs: mergeAttrs(pl.attrs, args)}
}

// PrintfFunc is the shape of fmt.Printf/log.Printf-style logging functions.
type PrintfFunc func(format string, args ...any)

// NewPrintfLogger adapts printf-style functions to Logger. Pass nil for levels
// that should be discarded.
func NewPrintfLogger(debugf, infof, warnf, errorf PrintfFunc) Logger {
	return &printfLogger{
		debugf: debugf,
		infof:  infof,
		warnf:  warnf,
		errorf: errorf,
	}
}

type printfLogger struct {
	debugf PrintfFunc
	infof  PrintfFunc
	warnf  PrintfFunc
	errorf PrintfFunc
	attrs  []any
}

func (pl *printfLogger) Debug(msg string, args ...any) { pl.printf(pl.debugf, msg, args...) }
func (pl *printfLogger) Info(msg string, args ...any)  { pl.printf(pl.infof, msg, args...) }
func (pl *printfLogger) Warn(msg string, args ...any)  { pl.printf(pl.warnf, msg, args...) }
func (pl *printfLogger) Error(msg string, args ...any) { pl.printf(pl.errorf, msg, args...) }

func (pl *printfLogger) printf(printf PrintfFunc, msg string, args ...any) {
	if printf == nil {
		return
	}
	printf("%s", FormatMessage(msg, mergeAttrs(pl.attrs, args)...))
}

func (pl *printfLogger) With(args ...any) Logger {
	return &printfLogger{
		debugf: pl.debugf,
		infof:  pl.infof,
		warnf:  pl.warnf,
		errorf: pl.errorf,
		attrs:  mergeAttrs(pl.attrs, args),
	}
}

func mergeAttrs(prefix, args []any) []any {
	if len(prefix) == 0 {
		return args
	}
	merged := make([]any, 0, len(prefix)+len(args))
	merged = append(merged, prefix...)
	merged = append(merged, args...)
	return merged
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

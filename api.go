package cfggo

import (
	"io"

	"github.com/iqhive/cfggo/cfglogger"
)

// Init initialises a configuration struct that embeds cfggo.Structure.
// It is a convenience wrapper so callers can write:
//
//	cfggo.Init(config, cfggo.WithFileConfig("config.json"))
//
// instead of the equivalent method call:
//
//	config.Init(config, cfggo.WithFileConfig("config.json"))
func Init(parent interface{}, options ...Option) {
	type initer interface {
		Init(interface{}, ...Option)
	}
	if i, ok := parent.(initer); ok {
		i.Init(parent, options...)
	}
}

// --- Logging helpers ----------------------------------------------------------

// LogLevel represents the verbosity of the global logger.
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
	LogLevelNone
)

// ParseLogLevel converts a string (case-insensitive) to a LogLevel.
// Returns (level, true) on success and (LogLevelInfo, false) when unrecognised.
func ParseLogLevel(s string) (LogLevel, bool) {
	switch s {
	case "debug", "DEBUG":
		return LogLevelDebug, true
	case "info", "INFO":
		return LogLevelInfo, true
	case "warn", "WARN", "warning", "WARNING":
		return LogLevelWarn, true
	case "error", "ERROR":
		return LogLevelError, true
	case "fatal", "FATAL":
		return LogLevelFatal, true
	case "none", "NONE", "off", "OFF":
		return LogLevelNone, true
	}
	return LogLevelInfo, false
}

// currentLevel tracks the level used by the global level-filter wrapper so
// SetLogLevel and SetLogOutput can be combined without losing state.
var currentLevel = LogLevelInfo

// SetLogLevel adjusts the verbosity of the global Logger.
// Messages below the given level are silently discarded.
func SetLogLevel(level LogLevel) {
	currentLevel = level
	if ll, ok := Logger.(*levelLogger); ok {
		ll.level = level
	} else {
		Logger = &levelLogger{inner: Logger, level: level}
	}
}

// SetLogOutput redirects global log output to w.
func SetLogOutput(w io.Writer) {
	inner := cfglogger.NewDefaultLoggerWithWriter(w)
	Logger = &levelLogger{inner: inner, level: currentLevel}
}

// levelLogger wraps a cfglogger.Logger and suppresses messages below a
// threshold level.
type levelLogger struct {
	inner cfglogger.Logger
	level LogLevel
}

func (l *levelLogger) Debug(msg string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		l.inner.Debug(msg, args...)
	}
}
func (l *levelLogger) Debugf(format string, args ...interface{}) {
	if l.level <= LogLevelDebug {
		l.inner.Debugf(format, args...)
	}
}
func (l *levelLogger) Info(msg string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		l.inner.Info(msg, args...)
	}
}
func (l *levelLogger) Infof(format string, args ...interface{}) {
	if l.level <= LogLevelInfo {
		l.inner.Infof(format, args...)
	}
}
func (l *levelLogger) Warn(msg string, args ...interface{}) {
	if l.level <= LogLevelWarn {
		l.inner.Warn(msg, args...)
	}
}
func (l *levelLogger) Warnf(format string, args ...interface{}) {
	if l.level <= LogLevelWarn {
		l.inner.Warnf(format, args...)
	}
}
func (l *levelLogger) Error(msg string, args ...interface{}) {
	if l.level <= LogLevelError {
		l.inner.Error(msg, args...)
	}
}
func (l *levelLogger) Errorf(format string, args ...interface{}) {
	if l.level <= LogLevelError {
		l.inner.Errorf(format, args...)
	}
}
func (l *levelLogger) Fatal(msg string, args ...interface{}) {
	if l.level <= LogLevelFatal {
		l.inner.Fatal(msg, args...)
	}
}
func (l *levelLogger) Fatalf(format string, args ...interface{}) {
	if l.level <= LogLevelFatal {
		l.inner.Fatalf(format, args...)
	}
}

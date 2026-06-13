package cfggo

import (
	"io"
	"log/slog"

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

// slogLevel maps a cfggo LogLevel onto a slog.Level. Fatal sits just above
// Error, and None uses a level high enough that nothing is ever emitted.
func (l LogLevel) slogLevel() slog.Level {
	switch l {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelInfo:
		return slog.LevelInfo
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	case LogLevelFatal:
		return slog.LevelError + 4
	case LogLevelNone:
		return slog.Level(1 << 30)
	}
	return slog.LevelInfo
}

// currentLevel tracks the configured level so SetLogLevel and SetLogOutput can
// be combined without losing state.
var currentLevel = LogLevelInfo

// SetLogLevel adjusts the verbosity of the global Logger. Messages below the
// given level are discarded by the underlying slog handler.
//
// This only affects the built-in DefaultLogger. If you supply your own logger
// (via the global Logger variable or WithLogger), that logger controls its own
// level and SetLogLevel is a no-op for it.
func SetLogLevel(level LogLevel) {
	currentLevel = level
	if dl, ok := Logger.(*cfglogger.DefaultLogger); ok {
		dl.SetLevel(level.slogLevel())
	}
}

// SetLogOutput redirects global log output to w, preserving the current level.
// Call this once at startup, before spawning goroutines that log.
func SetLogOutput(w io.Writer) {
	dl := cfglogger.NewDefaultLoggerWithWriter(w)
	dl.SetLevel(currentLevel.slogLevel())
	Logger = dl
}

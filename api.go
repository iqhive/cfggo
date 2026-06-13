package cfggo

import (
	"io"
	"log/slog"
	"sync/atomic"

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
func Init(parent interface{}, options ...Option) error {
	type initer interface {
		Init(interface{}, ...Option) error
	}
	if i, ok := parent.(initer); ok {
		return i.Init(parent, options...)
	}
	return nil
}

// --- Logging helpers ----------------------------------------------------------

// LogLevel represents the verbosity of the global logger.
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
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
	case "none", "NONE", "off", "OFF":
		return LogLevelNone, true
	}
	return LogLevelInfo, false
}

// slogLevel maps a cfggo LogLevel onto a slog.Level. None uses a level high
// enough that nothing is ever emitted
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
	case LogLevelNone:
		return slog.Level(1 << 30)
	}
	return slog.LevelInfo
}

// currentLevel tracks the configured level so SetLogLevel and SetLogOutput can
// be combined without losing state
// Store atomically because both setters may be called from different goroutines
var currentLevel atomic.Int32

func init() { currentLevel.Store(int32(LogLevelInfo)) }

// SetLogLevel adjusts the verbosity of the global logger. Messages below the
// given level are discarded by the underlying handler.
//
// SetLogLevel forwards the level to any global logger that implements
// cfglogger.LevelSetter (the built-in DefaultLogger does). If you supply a
// custom logger that does not implement LevelSetter, that logger controls its
// own level and SetLogLevel only records the level for a later SetLogOutput
func SetLogLevel(level LogLevel) {
	currentLevel.Store(int32(level))
	if ls, ok := GlobalLogger().(cfglogger.LevelSetter); ok {
		ls.SetLevel(level.slogLevel())
	}
}

// SetLogOutput redirects global log output to w, preserving the current level.
// Call this once at startup, before spawning goroutines that log.
func SetLogOutput(w io.Writer) {
	dl := cfglogger.NewDefaultLoggerWithWriter(w)
	dl.SetLevel(LogLevel(currentLevel.Load()).slogLevel())
	SetGlobalLogger(dl)
}

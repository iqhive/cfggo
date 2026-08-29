package cfggo

import (
	"io"
	"log/slog"
	"reflect"
	"strings"
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
	if parent == nil || isNilInterfaceValue(parent) {
		return GlobalErrorWrapper()(nil, ErrCodeInvalidArgument,
			"cfggo.Init: parent must not be nil")
	}
	type initer interface {
		Init(interface{}, ...Option) error
	}
	i, ok := parent.(initer)
	if !ok {
		// A struct that does not embed cfggo.Structure has no Init method, so
		// silently returning nil would leave the program completely
		// unconfigured while reporting success. Fail loudly instead
		return GlobalErrorWrapper()(nil, ErrCodeInvalidArgument,
			"cfggo.Init: parent (%T) must embed cfggo.Structure", parent)
	}
	return i.Init(parent, options...)
}

func isNilInterfaceValue(v interface{}) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
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
	LogLevelNone
)

// ParseLogLevel converts a string (case-insensitive) to a LogLevel.
// Returns (level, true) on success and (LogLevelInfo, false) when unrecognised.
func ParseLogLevel(s string) (LogLevel, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LogLevelDebug, true
	case "info":
		return LogLevelInfo, true
	case "warn", "warning":
		return LogLevelWarn, true
	case "error":
		return LogLevelError, true
	case "none", "off":
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

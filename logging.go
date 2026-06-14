package cfggo

import (
	"log/slog"
	"sync/atomic"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
)

// globalLogger and globalErrorWrapper hold the process-wide defaults used to
// seed Structure instances that have not been given a per-instance logger or
// wrapper, and for the handful of package-level helpers that run before a
// Structure exists
//
// They are stored behind atomic.Pointer rather than plain package variables so
// that SetGlobalLogger / SetLogOutput can be called concurrently with logging
// without triggering a data race (the previous exported `Logger` / `ErrorWrapper`
// variables could not be mutated safely once goroutines were logging)
var (
	globalLogger       atomic.Pointer[cfglogger.Logger]
	globalErrorWrapper atomic.Pointer[cfgerror.Wrapper]
)

func init() {
	SetGlobalLogger(cfglogger.NewDefaultLogger())
	SetGlobalErrorWrapper(cfgerror.NewDefaultWrapper())
}

// GlobalLogger returns the process-wide logger used to seed new Structure
// instances and the package-level helpers. It never returns nil.
//
// Replace it at startup with SetGlobalLogger to redirect all cfggo log output.
// Once an instance has been initialised it logs through its own (possibly
// inherited) logger, so swapping the global after Init only affects newly
// created instances and the package-level helpers
func GlobalLogger() cfglogger.Logger {
	if p := globalLogger.Load(); p != nil {
		return *p
	}
	return cfglogger.NewDefaultLogger()
}

// SetGlobalLogger replaces the process-wide logger. It is safe to call
// concurrently with logging
// A nil logger is ignored
func SetGlobalLogger(l cfglogger.Logger) {
	if l == nil {
		return
	}
	globalLogger.Store(&l)
}

// GlobalErrorWrapper returns the process-wide error wrapper used to seed new
// Structure instances. NB: this never returns nil
func GlobalErrorWrapper() cfgerror.Wrapper {
	if p := globalErrorWrapper.Load(); p != nil {
		return *p
	}
	return cfgerror.NewDefaultWrapper()
}

// SetGlobalErrorWrapper replaces the process-wide error wrapper. It is safe to
// call concurrently. NB:nil wrappers are ignored
func SetGlobalErrorWrapper(w cfgerror.Wrapper) {
	if w == nil {
		return
	}
	globalErrorWrapper.Store(&w)
}

// log returns the logger this instance should use: its own logger when set,
// otherwise the global logger. Internal cfggo code logs through
// this so that a per-instance WithLogger is consistently honoured
func (c *Structure) log() cfglogger.Logger {
	if c != nil && c.logger != nil {
		return c.logger
	}
	return GlobalLogger()
}

// bindConfigName returns a child logger with the configuration's name attached
// as a "config" attribute, so every line a Structure emits is attributable to
// that instance without each call site repeating the name
// It is called once during Init() - Loggers that cannot attach attributes are
// returned unchanged.
// The built-in DefaultLogger and a raw *slog.Logger are both handled
func bindConfigName(l cfglogger.Logger, name string) cfglogger.Logger {
	if l == nil || name == "" {
		return l
	}
	switch v := l.(type) {
	case cfglogger.WithAttrer:
		return v.With("config", name)
	case interface {
		With(args ...any) *slog.Logger
	}:
		// *slog.Logger.With returns *slog.Logger, which already satisfies
		// cfglogger.Logger, so a logger passed as slog.Default() still benefits
		return v.With("config", name)
	}
	return l
}

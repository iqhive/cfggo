package cfggo

import (
	"fmt"
	"sync/atomic"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
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
	globalErrorWrapper atomic.Pointer[errwrapper.ErrorWrapper]
)

func init() {
	SetGlobalLogger(cfglogger.NewDefaultLogger())
	SetGlobalErrorWrapper(errwrapper.NewDefaultErrorWrapper())
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
func GlobalErrorWrapper() errwrapper.ErrorWrapper {
	if p := globalErrorWrapper.Load(); p != nil {
		return *p
	}
	return errwrapper.NewDefaultErrorWrapper()
}

// SetGlobalErrorWrapper replaces the process-wide error wrapper. It is safe to
// call concurrently. NB:nil wrappers are ignored
func SetGlobalErrorWrapper(w errwrapper.ErrorWrapper) {
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

// The logXxxf helpers preserve cfggo's historical printf-style internal log
// calls on top of the slimmer structured Logger interface: they pre-format the
// message and emit it through the instance logger with no slog attributes.
//
// Deprecated: prefer logging through c.log() with structured slog key/value
// attributes (e.g. c.log().Warn("msg", "key", k, "err", err)) so output behind
// a structured handler stays queryable. These remain only for the lower-value
// internal call sites that have not yet been migrated
func (c *Structure) logDebugf(format string, args ...any) {
	c.log().Debug(fmt.Sprintf(format, args...))
}
func (c *Structure) logInfof(format string, args ...any) { c.log().Info(fmt.Sprintf(format, args...)) }
func (c *Structure) logWarnf(format string, args ...any) { c.log().Warn(fmt.Sprintf(format, args...)) }
func (c *Structure) logErrorf(format string, args ...any) {
	c.log().Error(fmt.Sprintf(format, args...))
}

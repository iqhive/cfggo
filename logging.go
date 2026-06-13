package cfggo

import (
	"fmt"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
)

// Logger is the global logger used to seed Structure instances that have not
// been given a per-instance logger via WithLogger or SetLogger, and for the
// handful of package-level helpers that run before a Structure exists.
//
// Replace it at startup to redirect all cfggo log output. Once an instance has
// been initialised it logs through its own (possibly inherited) logger, so
// swapping this variable after Init only affects newly created instances and
// the package-level helpers.
var Logger cfglogger.Logger = cfglogger.NewDefaultLogger()

// ErrorWrapper is the global error wrapper used to seed Structure instances
// that have not been given a per-instance wrapper via WithErrorWrapper.
var ErrorWrapper errwrapper.ErrorWrapper = errwrapper.NewDefaultErrorWrapper()

// log returns the logger this instance should use: its own logger when set,
// otherwise the global Logger. Internal cfggo code logs through this so that a
// per-instance WithLogger is consistently honoured.
func (c *Structure) log() cfglogger.Logger {
	if c != nil && c.logger != nil {
		return c.logger
	}
	return Logger
}

// The logXxxf helpers preserve cfggo's historical printf-style internal log
// calls on top of the slimmer structured Logger interface: they pre-format the
// message and emit it through the instance logger with no slog attributes.
func (c *Structure) logDebugf(format string, args ...any) { c.log().Debug(fmt.Sprintf(format, args...)) }
func (c *Structure) logInfof(format string, args ...any)  { c.log().Info(fmt.Sprintf(format, args...)) }
func (c *Structure) logWarnf(format string, args ...any)  { c.log().Warn(fmt.Sprintf(format, args...)) }
func (c *Structure) logErrorf(format string, args ...any) { c.log().Error(fmt.Sprintf(format, args...)) }

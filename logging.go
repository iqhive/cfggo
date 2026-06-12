package cfggo

import (
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
)

// Logger is the global logger used by all Structure instances that have not
// been given a per-instance logger via WithLogger or SetLogger.
// Replace it at startup to redirect all cfggo log output.
var Logger cfglogger.Logger = cfglogger.NewDefaultLogger()

// ErrorWrapper is the global error wrapper used by all Structure instances
// that have not been given a per-instance wrapper via WithErrorWrapper.
var ErrorWrapper errwrapper.ErrorWrapper = errwrapper.NewDefaultErrorWrapper()

// globalErrorWrapper adapts the package-level ErrorWrapper variable to the
// internal ErrorWrapper interface, so that the internal converter always
// respects whatever error-wrapping strategy the caller has configured.
type globalErrorWrapper struct{}

func (g *globalErrorWrapper) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	return ErrorWrapper(err, errorcode, msg, args...)
}

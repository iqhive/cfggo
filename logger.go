package cfggo

import (
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
)

var (
	ErrorWrapper errwrapper.ErrorWrapper = errwrapper.NewDefaultErrorWrapper()
	Logger       cfglogger.Logger        = cfglogger.NewDefaultLogger()
)

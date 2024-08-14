package cfggo

import (
	"bitbucket.org/iqhive/apierror/v3"
	"bitbucket.org/iqhive/iqlog/v3"
)

type errorWrapper func(err error, errorcode int, msg string, args ...interface{}) error

type logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

var ErrorWrapper errorWrapper = apierror.NewIfError

// var Logger logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
var Logger logger = iqlog.GlobalLogger

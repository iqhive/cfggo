package cfggo

import (
	"fmt"
	"log/slog"
	"os"
)

type errorWrapper func(err error, errorcode int, msg string, args ...interface{}) error

type logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

var (
	ErrorWrapper errorWrapper = DefaultErrorWrapper
	Logger       logger       = slog.New(slog.NewTextHandler(os.Stdout, nil))
)

func DefaultErrorWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	return fmt.Errorf(msg, args...)
}

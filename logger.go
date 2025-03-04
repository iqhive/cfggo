package cfggo

import (
	"fmt"
	"log/slog"
	"os"
)

type errorWrapper func(err error, errorcode int, msg string, args ...interface{}) error

type logger interface {
	Debug(msg string, args ...interface{})
	Debugf(format string, args ...interface{})
	Info(msg string, args ...interface{})
	Infof(format string, args ...interface{})
	Warn(msg string, args ...interface{})
	Warnf(format string, args ...interface{})
	Error(msg string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(msg string, args ...interface{})
	Fatalf(format string, args ...interface{})
}

var (
	ErrorWrapper errorWrapper = defaultErrorWrapper
	Logger       logger       = NewDefaultLogger()
)

func defaultErrorWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	return fmt.Errorf(msg, args...)
}

// DefaultLogger is the default implementation of ExtendedLogger
type DefaultLogger struct {
	logger *slog.Logger
}

// NewDefaultLogger creates a new DefaultLogger with the specified level
func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
}

// Debug logs a debug message
func (dl *DefaultLogger) Debug(msg string, args ...interface{}) {
	dl.logger.Debug(msg, args...)
}
func (dl *DefaultLogger) Debugf(format string, args ...interface{}) {
	dl.logger.Debug(fmt.Sprintf(format, args...))
}

// Info logs an info message
func (dl *DefaultLogger) Info(msg string, args ...interface{}) {
	dl.logger.Info(msg, args...)
}
func (dl *DefaultLogger) Infof(format string, args ...interface{}) {
	dl.logger.Info(fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (dl *DefaultLogger) Warn(msg string, args ...interface{}) {
	dl.logger.Warn(msg, args...)
}
func (dl *DefaultLogger) Warnf(format string, args ...interface{}) {
	dl.logger.Warn(fmt.Sprintf(format, args...))
}

// Error logs an error message
func (dl *DefaultLogger) Error(msg string, args ...interface{}) {
	dl.logger.Error(msg, args...)
}
func (dl *DefaultLogger) Errorf(format string, args ...interface{}) {
	dl.logger.Error(fmt.Sprintf(format, args...))
}

// Fatal logs a fatal message and exits
func (dl *DefaultLogger) Fatal(msg string, args ...interface{}) {
	dl.logger.Error(msg, args...)
	os.Exit(1)
}
func (dl *DefaultLogger) Fatalf(format string, args ...interface{}) {
	dl.logger.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// NoopLogger is a logger that does nothing
type NoopLogger struct{}

// Debug does nothing
func (nl *NoopLogger) Debug(msg string, args ...interface{}) {}

// Info does nothing
func (nl *NoopLogger) Info(msg string, args ...interface{}) {}

// Warn does nothing
func (nl *NoopLogger) Warn(msg string, args ...interface{}) {}

// Error does nothing
func (nl *NoopLogger) Error(msg string, args ...interface{}) {}

// Fatal does nothing
func (nl *NoopLogger) Fatal(msg string, args ...interface{}) {
	os.Exit(1)
}

// Initialize the default logger
func init() {
	// Replace the basic logger with our extended logger
	Logger = NewDefaultLogger()
}

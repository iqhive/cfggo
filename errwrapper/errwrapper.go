package errwrapper

import (
	"fmt"
)

// Logger interface for error wrapper logging
type Logger interface {
	Error(msg string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// ErrorWrapper wraps errors with optional error codes and messages
type ErrorWrapper func(err error, errorcode int, msg string, args ...interface{}) error

// ErrorWrapperWithLogger wraps errors with logging capability
type ErrorWrapperWithLogger func(logger Logger, err error, errorcode int, msg string, args ...interface{}) error

// NewDefaultErrorWrapper creates a default error wrapper (backwards compatible)
func NewDefaultErrorWrapper() ErrorWrapper {
	return defaultErrorWrapper
}

// NewDefaultErrorWrapperWithLogger creates a default error wrapper that logs errors
func NewDefaultErrorWrapperWithLogger() ErrorWrapperWithLogger {
	return defaultErrorWrapperWithLogger
}

// defaultErrorWrapper is the backwards compatible error wrapper
func defaultErrorWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	if msg == "" {
		return err
	}
	return fmt.Errorf(msg, args...)
}

// defaultErrorWrapperWithLogger wraps errors and logs them
func defaultErrorWrapperWithLogger(logger Logger, err error, errorcode int, msg string, args ...interface{}) error {
	var finalErr error

	if msg == "" {
		finalErr = err
	} else {
		finalErr = fmt.Errorf(msg, args...)
	}

	// Log the error if we have a logger and an actual error occurred
	if logger != nil && finalErr != nil {
		if err != nil {
			// Log with the underlying error context
			logger.Errorf("Error (code %d): %v (underlying: %v)", errorcode, finalErr, err)
		} else {
			// Log just the wrapped error
			logger.Errorf("Error (code %d): %v", errorcode, finalErr)
		}
	}

	return finalErr
}

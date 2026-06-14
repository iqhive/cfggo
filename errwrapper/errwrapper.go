package errwrapper

import "github.com/iqhive/cfggo/cfgerror"

// Error is the structured error produced by cfggo's default wrappers.
//
// Deprecated: use cfgerror.Error.
type Error = cfgerror.Error

// ErrorWrapper wraps errors with an optional error code and message.
//
// Deprecated: use cfgerror.Wrapper.
type ErrorWrapper = cfgerror.Wrapper

// Code returns the application-defined error code carried by err, or 0 if err
// is nil or does not wrap an *Error.
//
// Deprecated: use cfgerror.Code.
func Code(err error) int {
	return cfgerror.Code(err)
}

// NewDefaultErrorWrapper returns the default error wrapper, which produces
// chain-preserving *Error values.
//
// Deprecated: use cfgerror.NewDefaultWrapper.
func NewDefaultErrorWrapper() ErrorWrapper {
	return cfgerror.NewDefaultWrapper()
}

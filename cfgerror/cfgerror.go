package cfgerror

import (
	"errors"
	"fmt"
	"strings"
)

// Error is the structured error produced by cfggo's default wrappers.
//
// It deliberately keeps the underlying cause reachable through the standard
// errors.Is / errors.As / Unwrap machinery while still rendering a clean,
// human-readable message. This is the best of both worlds for developers:
// programmatic inspection of the cause and code, plus a readable string.
type Error struct {
	// Code is an optional, application-defined error code. Zero means "unset"
	// and is omitted from the rendered message.
	Code int
	// Msg is a human-readable message. It may be empty when only an underlying
	// cause is being wrapped.
	Msg string
	// Err is the underlying cause, if any. It is exposed via Unwrap.
	Err error
}

// Error renders a clean, readable message of the form:
//
//	[code] message: cause
//
// where the code prefix is omitted when zero, and the message or cause are
// omitted when absent.
func (e *Error) Error() string {
	var b strings.Builder
	if e.Code != 0 {
		fmt.Fprintf(&b, "[%d] ", e.Code)
	}
	switch {
	case e.Msg != "" && e.Err != nil:
		b.WriteString(e.Msg)
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	case e.Msg != "":
		b.WriteString(e.Msg)
	case e.Err != nil:
		b.WriteString(e.Err.Error())
	default:
		b.WriteString("unspecified error")
	}
	return b.String()
}

// Unwrap exposes the underlying cause so errors.Is and errors.As traverse the
// chain.
func (e *Error) Unwrap() error { return e.Err }

// Code returns the application-defined error code carried by err, or 0 if err
// is nil or does not wrap an *Error. It is a convenience for callers that want
// to branch on the code without a manual errors.As.
func Code(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

// Wrapper wraps errors with an optional error code and message.
type Wrapper func(err error, errorcode int, msg string, args ...interface{}) error

// NewDefaultWrapper returns the default error wrapper, which produces
// chain-preserving *Error values.
func NewDefaultWrapper() Wrapper {
	return defaultWrapper
}

// newError builds an *Error, applying fmt formatting to msg only when args are
// supplied (so a message containing a literal % is never misinterpreted). It
// returns a nil error when there is genuinely nothing to report.
func newError(err error, errorcode int, msg string, args ...interface{}) error {
	if err == nil && msg == "" && errorcode == 0 {
		return nil
	}
	formatted := msg
	if msg != "" && len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}
	return &Error{Code: errorcode, Msg: formatted, Err: err}
}

func defaultWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	return newError(err, errorcode, msg, args...)
}

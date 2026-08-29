package conf

import (
	"errors"
	"fmt"
)

var (
	ErrSyntax        = errors.New("conf: invalid syntax")
	ErrInvalidJSON   = errors.New("conf: invalid canonical JSON")
	ErrDuplicateKey  = errors.New("conf: duplicate configuration key")
	ErrKeyConflict   = errors.New("conf: configuration key conflict")
	ErrLimitExceeded = errors.New("conf: limit exceeded")
	ErrReadOnly      = errors.New("conf: handler is read-only; use RewriteOnSave to permit deterministic rewriting")
)

// ParseError describes a source location without including the configuration
// value, which may be secret.
type ParseError struct {
	Filename  string
	Line      int
	Column    int
	FirstLine int
	Err       error
}

func (e *ParseError) Error() string {
	location := "conf"
	if e.Filename != "" {
		location = e.Filename
	}
	if e.Line > 0 {
		location += fmt.Sprintf(":%d", e.Line)
		if e.Column > 0 {
			location += fmt.Sprintf(":%d", e.Column)
		}
	}
	if e.FirstLine > 0 {
		return fmt.Sprintf("%s: %v (first defined on line %d)", location, e.Err, e.FirstLine)
	}
	return fmt.Sprintf("%s: %v", location, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

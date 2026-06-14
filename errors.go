package cfggo

import (
	"errors"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/validcfg"
)

// Sentinel errors exposed by cfggo. Errors returned from the package wrap these
// (via fmt/errors semantics), so callers can branch on them with errors.Is
// without matching on message strings:
//
//	if errors.Is(err, cfggo.ErrUnknownKey) { ... }
//
// The underlying cause (a strconv error, an I/O error, a validator error, ...)
// remains reachable through errors.As / Unwrap.
var (
	// ErrNoHandler is returned when an operation needs a ConfigHandler but none
	// was configured (e.g. loading with no source).
	ErrNoHandler = errors.New("cfggo: no configuration handler configured")

	// ErrSource wraps failures originating from a configuration source
	// (file/HTTP/env handler) while loading or saving.
	ErrSource = errors.New("cfggo: configuration source error")

	// ErrUnknownKey indicates a configuration key that does not exist in the
	// config struct.
	ErrUnknownKey = errors.New("cfggo: unknown configuration key")

	// ErrValidation matches any validation failure (see validcfg.ErrValidation).
	ErrValidation = validcfg.ErrValidation
)

// Error codes carried by the *cfgerror.Error values cfggo produces. They are
// surfaced via ErrorCode and exist so call sites use named, self-documenting
// values instead of bare integer literals. The numeric values intentionally
// mirror common HTTP-style semantics (bad request / not found / internal) for
// readers who find that familiar, but callers should branch on the constants
// (or the sentinel errors above), not the raw numbers
const (
	// ErrCodeNone means no specific code was attached.
	ErrCodeNone = 0
	// ErrCodeInvalidArgument indicates a caller-supplied argument or option was
	// invalid (e.g. a nil flag set, a pointer-to-pointer parent).
	ErrCodeInvalidArgument = 400
	// ErrCodeNotFound indicates a referenced configuration key does not exist.
	ErrCodeNotFound = 404
	// ErrCodeInternal indicates an unexpected internal failure.
	ErrCodeInternal = 500
)

// ErrorCode returns the application-defined error code carried by err, or 0 if
// err is nil or carries no code. It is a thin re-export of cfgerror.Code so
// callers do not need to import the cfgerror package.
func ErrorCode(err error) int {
	return cfgerror.Code(err)
}

// kindError tags a concrete cause with a sentinel "kind" while keeping a clean,
// single-line message. Both the kind and the cause are reachable via
// errors.Is / errors.As (through Unwrap []error).
type kindError struct {
	kind  error
	cause error
}

func (e *kindError) Error() string {
	return e.cause.Error()
}

func (e *kindError) Unwrap() []error {
	return []error{e.kind, e.cause}
}

// wrapKind tags cause with the sentinel kind. If cause is nil, kind is returned
// directly.
func wrapKind(kind, cause error) error {
	if cause == nil {
		return kind
	}
	return &kindError{kind: kind, cause: cause}
}

package validcfg

import (
	"errors"
	"fmt"
	"strings"
)

// ErrValidation is a sentinel that all validation failures match via
// errors.Is, so callers can branch on validation errors without depending on
// the concrete ValidationError / ValidationErrors types:
//
//	if errors.Is(err, validcfg.ErrValidation) { ... }
var ErrValidation = errors.New("validation failed")

// Validator is a function that validates a configuration value
type Validator func(interface{}) error

// ValidationError represents an error that occurred during validation
type ValidationError struct {
	Key string
	Err error
}

// Error implements the error interface
func (v ValidationError) Error() string {
	return fmt.Sprintf("validation failed for '%s': %v", v.Key, v.Err)
}

// Unwrap returns the underlying error
func (v ValidationError) Unwrap() error {
	return v.Err
}

// Is reports that a ValidationError matches ErrValidation, while still
// unwrapping to the underlying cause for more specific matches.
func (v ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// Is reports that a ValidationErrors matches ErrValidation.
func (v ValidationErrors) Is(target error) bool {
	return target == ErrValidation
}

// Error implements the error interface
func (v ValidationErrors) Error() string {
	if len(v) == 0 {
		return "no validation errors"
	}

	if len(v) == 1 {
		return v[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors occurred:\n", len(v)))
	for i, err := range v {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// Custom returns a validator that uses a custom function
func Custom(fn func(interface{}) error) Validator {
	return fn
}

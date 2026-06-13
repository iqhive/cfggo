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

// ValidationError represents an error that occurred during validation.
//
// Value and Source are optional provenance fields. When populated (cfggo sets
// them automatically), the rendered message names the offending value and where
// it came from — e.g. "validation failed for 'port' (value=99999, from flag):
// value must be between 1 and 65535" — which answers the first question a
// developer asks: which source set the bad value?
type ValidationError struct {
	Key string
	Err error
	// Value is the configuration value that failed validation, if known
	Value interface{}
	// Source is a human-readable provenance label (e.g. "flag", "env",
	// "file", "default"), if known.
	Source string
	// hasValue distinguishes a deliberately-set nil/zero Value from an unset one so
	// the message only includes value/source detail when provenance was actually supplied
	hasValue bool
}

// WithProvenance returns a copy of the ValidationError annotated with the value
// that failed and a human-readable source label. It is used by cfggo to enrich
// validator output with provenance
func (v ValidationError) WithProvenance(value interface{}, source string) ValidationError {
	v.Value = value
	v.Source = source
	v.hasValue = true
	return v
}

// Error implements the error interface
func (v ValidationError) Error() string {
	if v.hasValue {
		src := v.Source
		if src == "" {
			src = "unknown"
		}
		return fmt.Sprintf("validation failed for '%s' (value=%v, from %s): %v", v.Key, v.Value, src, v.Err)
	}
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

package validcfg

import (
	"fmt"
	"strings"
)

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

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

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

// Package convert exposes type-conversion utilities that were previously part
// of the root cfggo package.
//
// Deprecated: This package is an implementation detail of cfggo. Its symbols
// are re-exported from the root package for backward compatibility and will be
// removed in a future version. Use cfggo.Structure.Set to update configuration
// values rather than calling ConvertValue directly.
package convert

import (
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

// ErrorWrapper is implemented by any type that wraps errors with a numeric
// error code and a formatted message. It is identical to
// internal/convert.ErrorWrapper.
//
// Deprecated: use internal/convert.ErrorWrapper.
type ErrorWrapper = iconvert.ErrorWrapper

// ConvertValue converts value to the given targetType, using errorWrapper
// (which may be nil) to produce richer error messages.
//
// Deprecated: internal implementation detail; use cfggo.Structure.Set.
func ConvertValue(value interface{}, targetType reflect.Type, errorWrapper ErrorWrapper) (interface{}, error) {
	return iconvert.ConvertValue(value, targetType, errorWrapper)
}

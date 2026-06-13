package cfggo

import (
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

// ConvertValue converts a value to targetType.
//
// Deprecated: this is an internal implementation detail exposed for backward
// compatibility. Use cfggo.Structure.Set to update configuration values.
// This symbol will be removed in the next minor version.
func ConvertValue(value interface{}, targetType reflect.Type) (interface{}, error) {
	return iconvert.ConvertValue(value, targetType, nil)
}

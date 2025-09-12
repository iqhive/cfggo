package cfggo

import (
	"reflect"

	"github.com/iqhive/cfggo/convert"
)

// globalErrorWrapper implements the ErrorWrapper interface for global usage
type globalErrorWrapper struct{}

func (g globalErrorWrapper) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	return ErrorWrapper(err, errorcode, msg, args...)
}

// ConvertValue converts a value to the target type using the global error wrapper
// This function maintains backward compatibility
func ConvertValue(value interface{}, targetType reflect.Type) (interface{}, error) {
	wrapper := globalErrorWrapper{}
	return convert.ConvertValue(value, targetType, wrapper)
}

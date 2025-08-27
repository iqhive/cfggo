package cfggo

import (
	"encoding"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ConvertValue converts a value to the target type
// This function extracts common type conversion logic from set() and dynamicVar.Set()
func ConvertValue(value interface{}, targetType reflect.Type) (interface{}, error) {
	valueType := reflect.TypeOf(value)

	// If types already match, no conversion needed
	if valueType == targetType {
		return value, nil
	}

	valueValue := reflect.ValueOf(value)

	// Special handling for time.Duration
	if targetType == reflect.TypeOf(time.Duration(0)) {
		switch valueType.Kind() {
		case reflect.String:
			duration, err := time.ParseDuration(valueValue.String())
			if err == nil {
				return duration, nil
			}
			return nil, ErrorWrapper(err, 400, "Cannot convert string to duration: %v", err)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return time.Duration(valueValue.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return time.Duration(valueValue.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return time.Duration(int64(valueValue.Float())), nil
		}
	} else if targetType == reflect.TypeOf(int64(0)) && valueType != reflect.TypeOf(time.Duration(0)) {
		// Ensure int64 values don't get mistakenly converted to Duration
		switch valueType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return valueValue.Int(), nil
		case reflect.Float64:
			return int64(valueValue.Float()), nil
		case reflect.String:
			intVal, err := strconv.ParseInt(valueValue.String(), 10, 64)
			if err == nil {
				return intVal, nil
			}
			return nil, ErrorWrapper(err, 400, "Cannot convert string to int64: %v", err)
		}
	} else if targetType == reflect.TypeOf(time.Time{}) {
		// Handle time.Time conversion
		if valueType.Kind() == reflect.String {
			parsedTime, err := time.Parse(time.RFC3339, valueValue.String())
			if err == nil {
				return parsedTime, nil
			}
			return nil, ErrorWrapper(err, 400, "Cannot convert string to time.Time: %v", err)
		}
	}

	// Handle numeric type conversions
	if isNumericType(targetType) && isNumericType(valueType) {
		switch targetType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var intVal int64
			switch valueType.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal = valueValue.Int()
			case reflect.Float32, reflect.Float64:
				intVal = int64(valueValue.Float())
			case reflect.String:
				var err error
				intVal, err = strconv.ParseInt(valueValue.String(), 10, 64)
				if err != nil {
					return nil, ErrorWrapper(err, 400, "Cannot convert string to int: %v", err)
				}
			}
			newValue := reflect.New(targetType).Elem()
			newValue.SetInt(intVal)
			return newValue.Interface(), nil

		case reflect.Float32, reflect.Float64:
			var floatVal float64
			switch valueType.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				floatVal = float64(valueValue.Int())
			case reflect.Float32, reflect.Float64:
				floatVal = valueValue.Float()
			case reflect.String:
				var err error
				floatVal, err = strconv.ParseFloat(valueValue.String(), 64)
				if err != nil {
					return nil, ErrorWrapper(err, 400, "Cannot convert string to float: %v", err)
				}
			}
			newValue := reflect.New(targetType).Elem()
			newValue.SetFloat(floatVal)
			return newValue.Interface(), nil
		}
	}

	// Handle boolean conversions
	if targetType.Kind() == reflect.Bool && valueType.Kind() == reflect.String {
		strVal := strings.ToLower(valueValue.String())
		if strVal == "true" || strVal == "t" || strVal == "yes" || strVal == "y" || strVal == "1" {
			return true, nil
		} else if strVal == "false" || strVal == "f" || strVal == "no" || strVal == "n" || strVal == "0" {
			return false, nil
		}
		return nil, ErrorWrapper(nil, 400, "Cannot convert string '%s' to bool", strVal)
	}

	// Try standard conversion if types are convertible
	if valueType.ConvertibleTo(targetType) {
		return valueValue.Convert(targetType).Interface(), nil
	}

	if valueType.Kind() == reflect.String && targetType.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		newValue := reflect.New(targetType).Elem()
		if err := newValue.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(valueValue.String())); err != nil {
			return nil, ErrorWrapper(err, 400, "Cannot convert string to %v: %v", targetType, err)
		}
		return newValue.Interface(), nil
	}
	if valueType.Kind() == reflect.Slice && targetType.Kind() == reflect.Slice {
		newValue := reflect.MakeSlice(targetType, valueValue.Len(), valueValue.Cap())
		for i := range valueValue.Len() {
			elem, err := ConvertValue(valueValue.Index(i).Interface(), targetType.Elem())
			if err != nil {
				return nil, ErrorWrapper(err, 400, "Cannot convert slice element %d to %v: %v", i, targetType.Elem(), err)
			}
			newValue.Index(i).Set(reflect.ValueOf(elem))
		}
		return newValue.Interface(), nil
	}

	return nil, ErrorWrapper(nil, 400, "Type mismatch: %T cannot be converted to %v", value, targetType)
}

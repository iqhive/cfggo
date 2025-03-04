package cfggo

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type dynamicVar struct {
	config *Structure
	name   string
	want   reflect.Type
}

func (d *dynamicVar) Set(s string) error {
	if d.want == nil {
		return fmt.Errorf("dynamicVar has nil type")
	}

	var value = reflect.New(d.want).Elem()

	// Special case for interface{} type - try to preserve the original type
	if d.want.Kind() == reflect.Interface {
		// Try to parse as number first
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			// Check if it's actually an integer
			if float64(int(f)) == f {
				return d.config.Set(d.name, int(f))
			}
			return d.config.Set(d.name, f)
		}

		// Try to parse as bool
		if b, err := strconv.ParseBool(s); err == nil {
			return d.config.Set(d.name, b)
		}

		// Default to string
		return d.config.Set(d.name, s)
	}

	switch d.want.Kind() {
	case reflect.Bool:
		if len(s) == 0 {
			value.SetBool(false)
		} else {
			switch strings.ToLower(s)[0] {
			case '1', 't', 'y':
				value.SetBool(true)
			case '0', 'f', 'n':
				value.SetBool(false)
			default:
				return fmt.Errorf("invalid bool value %s (use true/false, yes/no, 1/0)", s)
			}
		}
	case reflect.String:
		value.SetString(s)
	case reflect.Struct:
		// Handle time.Time and other struct types that implement TextUnmarshaler or JSONUnmarshaler
		if d.want == reflect.TypeOf(time.Time{}) {
			parsedTime, err := time.Parse(time.RFC3339, s)
			if err != nil {
				// Try other common time formats
				for _, format := range []string{
					time.RFC3339Nano,
					time.RFC3339,
					"2006-01-02T15:04:05",
					"2006-01-02 15:04:05",
					"2006-01-02",
				} {
					if parsedTime, err = time.Parse(format, s); err == nil {
						break
					}
				}

				if err != nil {
					return fmt.Errorf("invalid time format: %s (expected RFC3339 format like '2006-01-02T15:04:05Z')", s)
				}
			}
			value.Set(reflect.ValueOf(parsedTime))
		} else {
			// Try TextUnmarshaler first
			unmarshaler, ok := value.Addr().Interface().(encoding.TextUnmarshaler)
			if ok {
				if err := unmarshaler.UnmarshalText([]byte(s)); err != nil {
					return fmt.Errorf("failed to unmarshal text: %v", err)
				}
				return d.config.Set(d.name, value.Interface())
			}

			// Then try JSONUnmarshaler
			jsonUnmarshaler, ok := value.Addr().Interface().(json.Unmarshaler)
			if ok {
				if err := jsonUnmarshaler.UnmarshalJSON([]byte(s)); err != nil {
					return fmt.Errorf("failed to unmarshal JSON: %v", err)
				}
				return d.config.Set(d.name, value.Interface())
			}

			return fmt.Errorf("structs must support encoding.TextUnmarshaler or json.Unmarshaler")
		}
	case reflect.Int64:
		// Special case for time.Duration
		if d.want == reflect.TypeOf(time.Duration(0)) {
			parsedDuration, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("invalid duration format: %s (expected format like '1h30m', '10s')", s)
			}
			value.Set(reflect.ValueOf(parsedDuration))
		} else {
			// Regular int64
			parsedInt, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid int64 value: %s (expected a number)", s)
			}
			value.SetInt(parsedInt)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		parsedInt, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid int value: %s (expected a number)", s)
		}

		// Check for overflow
		switch d.want.Kind() {
		case reflect.Int8:
			if parsedInt < math.MinInt8 || parsedInt > math.MaxInt8 {
				return fmt.Errorf("int8 overflow: %d (range: %d to %d)", parsedInt, math.MinInt8, math.MaxInt8)
			}
		case reflect.Int16:
			if parsedInt < math.MinInt16 || parsedInt > math.MaxInt16 {
				return fmt.Errorf("int16 overflow: %d (range: %d to %d)", parsedInt, math.MinInt16, math.MaxInt16)
			}
		case reflect.Int32:
			if parsedInt < math.MinInt32 || parsedInt > math.MaxInt32 {
				return fmt.Errorf("int32 overflow: %d (range: %d to %d)", parsedInt, math.MinInt32, math.MaxInt32)
			}
		}

		value.SetInt(parsedInt)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsedUint, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned int value: %s (expected a positive number)", s)
		}

		// Check for overflow
		switch d.want.Kind() {
		case reflect.Uint8:
			if parsedUint > math.MaxUint8 {
				return fmt.Errorf("uint8 overflow: %d (max: %d)", parsedUint, math.MaxUint8)
			}
		case reflect.Uint16:
			if parsedUint > math.MaxUint16 {
				return fmt.Errorf("uint16 overflow: %d (max: %d)", parsedUint, math.MaxUint16)
			}
		case reflect.Uint32:
			if parsedUint > math.MaxUint32 {
				return fmt.Errorf("uint32 overflow: %d (max: %d)", parsedUint, math.MaxUint32)
			}
		}

		value.SetUint(parsedUint)
	case reflect.Slice:
		// Handle empty string case for slices
		if s == "" {
			value.Set(reflect.MakeSlice(d.want, 0, 0))
			return d.config.Set(d.name, value.Interface())
		}

		split := strings.Split(s, ",")
		value.Set(reflect.MakeSlice(d.want, len(split), len(split)))

		for i, v := range split {
			v = strings.TrimSpace(v)
			elemValue := value.Index(i)

			// Handle different element types
			switch elemValue.Kind() {
			case reflect.String:
				elemValue.SetString(v)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal, err := strconv.ParseInt(v, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid int in slice at position %d: %s", i, v)
				}
				elemValue.SetInt(intVal)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintVal, err := strconv.ParseUint(v, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid uint in slice at position %d: %s", i, v)
				}
				elemValue.SetUint(uintVal)
			case reflect.Float32, reflect.Float64:
				floatVal, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return fmt.Errorf("invalid float in slice at position %d: %s", i, v)
				}
				elemValue.SetFloat(floatVal)
			case reflect.Bool:
				boolVal, err := strconv.ParseBool(v)
				if err != nil {
					return fmt.Errorf("invalid bool in slice at position %d: %s", i, v)
				}
				elemValue.SetBool(boolVal)
			default:
				return fmt.Errorf("unsupported slice element type: %s", elemValue.Kind())
			}
		}
	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: %s", s)
		}
		value.SetFloat(floatVal)
	case reflect.Map:
		// Handle empty string case for maps
		if s == "" {
			value.Set(reflect.MakeMap(d.want))
			return d.config.Set(d.name, value.Interface())
		}

		split := strings.Split(s, ",")
		value.Set(reflect.MakeMap(d.want))

		for _, v := range split {
			v = strings.TrimSpace(v)
			kv := strings.SplitN(v, ":", 2)
			if len(kv) != 2 {
				return fmt.Errorf("invalid map entry %s (expected format 'key:value')", v)
			}

			key := reflect.New(d.want.Key()).Elem()
			val := reflect.New(d.want.Elem()).Elem()

			// Handle different key types
			keyStr := strings.TrimSpace(kv[0])
			switch key.Kind() {
			case reflect.String:
				key.SetString(keyStr)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal, err := strconv.ParseInt(keyStr, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid int key in map: %s", keyStr)
				}
				key.SetInt(intVal)
			default:
				return fmt.Errorf("unsupported map key type: %s", key.Kind())
			}

			// Handle different value types
			valStr := strings.TrimSpace(kv[1])
			switch val.Kind() {
			case reflect.String:
				val.SetString(valStr)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal, err := strconv.ParseInt(valStr, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid int value in map: %s", valStr)
				}
				val.SetInt(intVal)
			case reflect.Bool:
				boolVal, err := strconv.ParseBool(valStr)
				if err != nil {
					return fmt.Errorf("invalid bool value in map: %s", valStr)
				}
				val.SetBool(boolVal)
			case reflect.Float32, reflect.Float64:
				floatVal, err := strconv.ParseFloat(valStr, 64)
				if err != nil {
					return fmt.Errorf("invalid float value in map: %s", valStr)
				}
				val.SetFloat(floatVal)
			default:
				return fmt.Errorf("unsupported map value type: %s", val.Kind())
			}

			value.SetMapIndex(key, val)
		}
	default:
		return fmt.Errorf("unsupported type %s", d.want.Kind())
	}

	return d.config.Set(d.name, value.Interface())
}

func (d *dynamicVar) String() string {
	if d == nil || d.config == nil {
		return ""
	}
	val, ok := d.config.Get(d.name)
	if !ok {
		return ""
	}
	return fmt.Sprint(val)
}

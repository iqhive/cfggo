package convert

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ErrorWrapper is satisfied by *cfggo.Structure (via WrapError) and by any
// concrete type that wraps errors with a numeric code and a formatted message.
// Passing nil is valid: errors are returned unwrapped.
type ErrorWrapper interface {
	WrapError(err error, errorcode int, msg string, args ...interface{}) error
}

var (
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	timeDurationType    = reflect.TypeOf(time.Duration(0))
	timeTimeType        = reflect.TypeOf(time.Time{})
)

// wrapErr returns a formatted error, optionally wrapping it through ew.
func wrapErr(ew ErrorWrapper, cause error, code int, msg string, args ...interface{}) error {
	if ew != nil {
		return ew.WrapError(cause, code, msg, args...)
	}
	if msg != "" {
		return fmt.Errorf(msg, args...)
	}
	return cause
}

// ConvertString converts the string s to the given target type.
// This is the canonical parser for values arriving as strings: flag values,
// environment variables, and `default` struct tags.
func ConvertString(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	// Empty string for bool is false (not an error), because flag packages can
	// invoke Set("") for bare boolean flags.
	if target.Kind() == reflect.Bool && s == "" {
		return false, nil
	}

	// Time types first so they are not caught by the int64/float64 branches.
	switch target {
	case timeDurationType:
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse duration %q: %v", s, err)
		}
		return d, nil
	case timeTimeType:
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse time %q: %v", s, err)
		}
		return t, nil
	}

	switch target.Kind() {
	case reflect.Bool:
		switch strings.ToLower(s) {
		case "true", "t", "yes", "y", "1":
			return true, nil
		case "false", "f", "no", "n", "0":
			return false, nil
		default:
			return nil, wrapErr(ew, nil, 400, "cannot parse bool %q", s)
		}

	case reflect.String:
		return s, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		bitSize := int(target.Size() * 8)
		v, err := strconv.ParseInt(s, 10, bitSize)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse int %q: %v", s, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		bitSize := int(target.Size() * 8)
		v, err := strconv.ParseUint(s, 10, bitSize)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse uint %q: %v", s, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Float32, reflect.Float64:
		bitSize := 64
		if target.Kind() == reflect.Float32 {
			bitSize = 32
		}
		v, err := strconv.ParseFloat(s, bitSize)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse float %q: %v", s, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Slice:
		return convertStringToSlice(s, target, ew)

	case reflect.Map:
		return convertStringToMap(s, target, ew)

	default:
		// Prefer pointer-receiver TextUnmarshaler (the common Go convention).
		ptrType := reflect.PointerTo(target)
		if ptrType.Implements(textUnmarshalerType) {
			ptr := reflect.New(target)
			if err := ptr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
				return nil, wrapErr(ew, err, 400, "UnmarshalText(%v) failed: %v", target, err)
			}
			return ptr.Elem().Interface(), nil
		}
		// Value-receiver TextUnmarshaler.
		if target.Implements(textUnmarshalerType) {
			v := reflect.New(target).Elem()
			if err := v.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
				return nil, wrapErr(ew, err, 400, "UnmarshalText(%v) failed: %v", target, err)
			}
			return v.Interface(), nil
		}
		// JSON fallback for structs, maps, etc.
		ptr := reflect.New(target)
		if err := json.Unmarshal([]byte(s), ptr.Interface()); err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse %q as %v", s, target)
		}
		return ptr.Elem().Interface(), nil
	}
}

// convertStringToSlice parses s as a JSON array or comma-separated list.
func convertStringToSlice(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	elemType := target.Elem()

	// JSON array.
	if strings.HasPrefix(s, "[") {
		slice := reflect.New(target).Elem()
		if err := json.Unmarshal([]byte(s), slice.Addr().Interface()); err == nil {
			return slice.Interface(), nil
		}
	}

	if s == "" {
		return reflect.MakeSlice(target, 0, 0).Interface(), nil
	}

	// Comma-separated values.
	parts := strings.Split(s, ",")
	slice := reflect.MakeSlice(target, len(parts), len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		elem, err := ConvertString(p, elemType, ew)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot convert slice[%d] %q: %v", i, p, err)
		}
		slice.Index(i).Set(reflect.ValueOf(elem))
	}
	return slice.Interface(), nil
}

// convertStringToMap parses s as a JSON object or "k:v,k:v" pairs.
func convertStringToMap(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	keyType := target.Key()
	valType := target.Elem()

	// JSON object.
	if strings.HasPrefix(s, "{") {
		m := reflect.MakeMap(target)
		if err := json.Unmarshal([]byte(s), m.Addr().Interface()); err == nil {
			return m.Interface(), nil
		}
	}

	// "key:val,key:val" format.
	m := reflect.MakeMap(target)
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) != 2 {
			return nil, wrapErr(ew, nil, 400, "invalid map entry %q: expected key:value", pair)
		}
		k, err := ConvertString(strings.TrimSpace(kv[0]), keyType, ew)
		if err != nil {
			return nil, err
		}
		v, err := ConvertString(strings.TrimSpace(kv[1]), valType, ew)
		if err != nil {
			return nil, err
		}
		m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
	}
	return m.Interface(), nil
}

// ConvertValue coerces value (any Go type) to the target reflect.Type.
// It is used by Structure.set() to normalise values arriving from JSON, env
// vars, or command-line flags after they have already been parsed into a Go value.
func ConvertValue(value interface{}, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	srcType := reflect.TypeOf(value)

	// Fast path: identical or directly assignable types.
	if srcType == target {
		return value, nil
	}
	if srcType.AssignableTo(target) {
		return value, nil
	}

	// target is empty interface, accept anything.
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return value, nil
	}

	// String source: delegate to ConvertString which knows all the formats.
	if srcType.Kind() == reflect.String {
		return ConvertString(value.(string), target, ew)
	}

	// time.Duration target with a numeric source.
	if target == timeDurationType {
		src := reflect.ValueOf(value)
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return time.Duration(src.Int()), nil
		case reflect.Float32, reflect.Float64:
			return time.Duration(int64(src.Float())), nil
		}
	}

	// Numeric to numeric.
	if isNumeric(srcType) && isNumeric(target) {
		return convertNumeric(reflect.ValueOf(value), target)
	}

	// Slice to slice (element-wise, e.g. []interface{} to []string from JSON).
	if srcType.Kind() == reflect.Slice && target.Kind() == reflect.Slice {
		return convertSlice(reflect.ValueOf(value), target, ew)
	}

	// Map to map (element-wise, e.g. map[string]interface{} to map[string]string).
	if srcType.Kind() == reflect.Map && target.Kind() == reflect.Map {
		return convertMap(reflect.ValueOf(value), target, ew)
	}

	// Standard reflect conversion (handles same-kind numeric aliases, etc.).
	if srcType.ConvertibleTo(target) {
		return reflect.ValueOf(value).Convert(target).Interface(), nil
	}

	// JSON marshal+unmarshal as a last resort.
	b, err := json.Marshal(value)
	if err != nil {
		return nil, wrapErr(ew, err, 400, "cannot marshal %T to JSON", value)
	}
	ptr := reflect.New(target)
	if err := json.Unmarshal(b, ptr.Interface()); err != nil {
		return nil, wrapErr(ew, nil, 400, "type mismatch: %T cannot be converted to %v", value, target)
	}
	return ptr.Elem().Interface(), nil
}

func isNumeric(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func convertNumeric(src reflect.Value, target reflect.Type) (interface{}, error) {
	out := reflect.New(target).Elem()
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out.SetInt(src.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out.SetInt(int64(src.Uint()))
		case reflect.Float32, reflect.Float64:
			out.SetInt(int64(src.Float()))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out.SetUint(uint64(src.Int()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out.SetUint(src.Uint())
		case reflect.Float32, reflect.Float64:
			out.SetUint(uint64(src.Float()))
		}
	case reflect.Float32, reflect.Float64:
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out.SetFloat(float64(src.Int()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out.SetFloat(float64(src.Uint()))
		case reflect.Float32, reflect.Float64:
			out.SetFloat(src.Float())
		}
	}
	return out.Interface(), nil
}

func convertSlice(src reflect.Value, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	elemType := target.Elem()
	out := reflect.MakeSlice(target, src.Len(), src.Len())
	for i := 0; i < src.Len(); i++ {
		elem, err := ConvertValue(src.Index(i).Interface(), elemType, ew)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot convert slice[%d]: %v", i, err)
		}
		out.Index(i).Set(reflect.ValueOf(elem))
	}
	return out.Interface(), nil
}

func convertMap(src reflect.Value, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	out := reflect.MakeMap(target)
	for _, k := range src.MapKeys() {
		ck, err := ConvertValue(k.Interface(), target.Key(), ew)
		if err != nil {
			return nil, err
		}
		cv, err := ConvertValue(src.MapIndex(k).Interface(), target.Elem(), ew)
		if err != nil {
			return nil, err
		}
		out.SetMapIndex(reflect.ValueOf(ck), reflect.ValueOf(cv))
	}
	return out.Interface(), nil
}

package convert

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	byteType            = reflect.TypeOf(byte(0))
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
// environment variables, and `default` struct tags. A type with its own
// UnmarshalText method (or whose pointer has one) parses itself, whatever its
// underlying kind; time.Duration and time.Time are the exceptions and keep
// their duration and RFC 3339 forms
func ConvertString(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	// Empty string for bool is false (not an error), because flag packages can
	// invoke Set("") for bare boolean flags.
	if target.Kind() == reflect.Bool && s == "" {
		return reflect.ValueOf(false).Convert(target).Interface(), nil
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

	// An empty interface target has no declared shape: infer the most specific
	// Go value the text describes (int, float64, bool, a JSON object or array,
	// or the string itself) so callers get a typed value rather than bare text
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return inferInterfaceValue(s), nil
	}

	// A type that parses its own text comes before the kind switch: a named
	// type whose underlying kind is a builtin (net.IP is a byte slice,
	// slog.Level an int, `type Colour string`) would otherwise be parsed as
	// that kind, turning "1.2.3.4" into a base64 error and "info" into an
	// integer one. The builtin kinds themselves have no methods and are
	// unaffected; time.Duration and time.Time were handled above
	if textUnmarshalerTarget(target) {
		return unmarshalText(s, target, ew)
	}

	switch target.Kind() {
	case reflect.Bool:
		// Convert to the target so a named bool type (type Toggle bool) gets a
		// Toggle, not a bare bool: the slice and map builders assign the result
		// with reflection, which panics on a type mismatch
		switch strings.ToLower(s) {
		case "true", "t", "yes", "y", "1":
			return reflect.ValueOf(true).Convert(target).Interface(), nil
		case "false", "f", "no", "n", "0":
			return reflect.ValueOf(false).Convert(target).Interface(), nil
		default:
			return nil, wrapErr(ew, nil, 400, "cannot parse bool %q", s)
		}

	case reflect.String:
		return reflect.ValueOf(s).Convert(target).Interface(), nil

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
		// ParseFloat accepts "NaN", "Inf", "infinity" and friends, but a
		// non-finite value cannot be written back out: json.Marshal (and so
		// Save) refuses it
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, wrapErr(ew, nil, 400, "cannot parse float %q: value must be finite", s)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Slice:
		return convertStringToSlice(s, target, ew)

	case reflect.Map:
		return convertStringToMap(s, target, ew)

	default:
		// JSON fallback for structs, pointers, arrays, non-empty interfaces
		// and anything else without a text form of its own
		ptr := reflect.New(target)
		if err := json.Unmarshal([]byte(s), ptr.Interface()); err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse %q as %v", s, target)
		}
		return ptr.Elem().Interface(), nil
	}
}

// textUnmarshalerTarget reports whether values of type target parse their own
// text: target or *target implements encoding.TextUnmarshaler. Builtin kinds
// have no methods and never qualify. A non-empty interface type is excluded
// even when it declares UnmarshalText itself (encoding.TextUnmarshaler, say):
// its zero value is a nil interface with no concrete type to unmarshal into,
// so calling the method would panic
func textUnmarshalerTarget(target reflect.Type) bool {
	if target.Kind() == reflect.Interface {
		return false
	}
	return target.Implements(textUnmarshalerType) || reflect.PointerTo(target).Implements(textUnmarshalerType)
}

// unmarshalText parses s with the target's UnmarshalText method. For a pointer
// target (func() *T where *T has UnmarshalText) a non-nil T is allocated and
// its address returned; calling the method on reflect.Zero(target) would use a
// nil receiver. Otherwise a fresh T is filled in through its address, which
// reaches a pointer-receiver method (the common Go convention) as well as a
// value-receiver one
func unmarshalText(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	elem := target
	if target.Kind() == reflect.Pointer {
		elem = target.Elem()
	}
	ptr := reflect.New(elem)
	if err := ptr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
		return nil, wrapErr(ew, err, 400, "UnmarshalText(%v) failed: %v", target, err)
	}
	if target.Kind() == reflect.Pointer {
		return ptr.Interface(), nil
	}
	return ptr.Elem().Interface(), nil
}

// inferInterfaceValue picks a Go value for text destined for an interface{}
// slot: an int when the text is an integer literal that fits (int64 beyond
// that), a float64 for other finite numbers (an integral float such as 1e3
// becomes an int; "NaN" and "Inf" stay text), a bool for true/false, a decoded
// JSON object or array, else the string
func inferInterfaceValue(s string) interface{} {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		if int64(int(i)) == i {
			return int(i)
		}
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		if isWholeFinite(f) && f >= -9223372036854775808.0 && f < 9223372036854775808.0 {
			if i := int64(f); int64(int(i)) == i {
				return int(i)
			}
		}
		return f
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if trimmed := strings.TrimSpace(s); strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var v interface{}
		dec := json.NewDecoder(strings.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&v); err == nil && !dec.More() {
			if normalised, err := NormalizeJSONNumbers(v); err == nil {
				return normalised
			}
		}
	}
	return s
}

// convertStringToSlice parses s as a JSON array or comma-separated list. A byte
// slice is the exception: its text form is base64, which is what encoding/json
// (and therefore Save) produces, so a saved value loads back unchanged.
func convertStringToSlice(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	elemType := target.Elem()

	// JSON array: decoded untyped with exact numbers, then converted element
	// by element, so `["1","2"]` reaches a []int as 1 and 2, a number in a
	// []string becomes its literal text, and an integer beyond 2^53 reaches a
	// []int64 or []interface{} unrounded. Text that starts with "[" is JSON or
	// nothing: malformed JSON, or an element that does not fit the target, is
	// an error rather than a comma-separated reading of the brackets
	if strings.HasPrefix(s, "[") {
		decoded, err := decodeJSONText(s)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse %q as a JSON array for %v: %v", s, target, err)
		}
		items, ok := decoded.([]interface{})
		if !ok {
			return nil, wrapErr(ew, nil, 400, "cannot parse %q as a JSON array for %v", s, target)
		}
		return convertSlice(reflect.ValueOf(items), target, ew)
	}

	if s == "" {
		return reflect.MakeSlice(target, 0, 0).Interface(), nil
	}

	if elemType.Kind() == reflect.Uint8 {
		return convertStringToBytes(s, target, ew)
	}

	// Comma-separated values.
	rawParts := strings.Split(s, ",")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	slice := reflect.MakeSlice(target, len(parts), len(parts))
	for i, p := range parts {
		elem, err := ConvertString(p, elemType, ew)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot convert slice[%d] %q: %v", i, p, err)
		}
		slice.Index(i).Set(valueForType(elem, elemType))
	}
	return slice.Interface(), nil
}

// convertStringToBytes decodes the text form of a []byte (or a slice of a
// named byte type): standard base64 with or without padding. A JSON array of
// numbers is handled by the caller. Raw text is deliberately not accepted, so
// a value that merely looks like base64 is never silently decoded into
// garbage; a field that holds free text should be a string
func convertStringToBytes(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		if decoded, err = base64.RawStdEncoding.DecodeString(s); err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse %v: text must be base64 (as Save writes it) or a JSON array of numbers", target)
		}
	}
	out := reflect.MakeSlice(target, len(decoded), len(decoded))
	if target.Elem() == byteType {
		reflect.Copy(out, reflect.ValueOf(decoded))
		return out.Interface(), nil
	}
	// reflect.Copy requires identical element types, so a slice of a named
	// byte type (type B byte; []B) is filled one element at a time
	for i, b := range decoded {
		out.Index(i).SetUint(uint64(b))
	}
	return out.Interface(), nil
}

// decodeJSONText decodes s, which starts with "[" or "{", into an untyped
// container ([]interface{} or map[string]interface{}) with numbers kept exact:
// a json.Decoder with UseNumber followed by NormalizeJSONNumbers, so an
// integer beyond 2^53 is not rounded through float64 on the way to a typed
// element. Data after the value is an error, as it is for json.Unmarshal
func decodeJSONText(s string) (interface{}, error) {
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON value")
	}
	return NormalizeJSONNumbers(v)
}

// convertStringToMap parses s as a JSON object or "k:v,k:v" pairs.
func convertStringToMap(s string, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	keyType := target.Key()
	valType := target.Elem()

	// JSON object: decoded untyped with exact numbers, then converted entry by
	// entry, so `{"a":"1"}` reaches a map[string]int as 1 and an integer beyond
	// 2^53 reaches a map[string]interface{} unrounded. Text that starts with
	// "{" is JSON or nothing: malformed JSON, or an entry that does not fit the
	// target, is an error rather than a key:value reading of the braces
	if strings.HasPrefix(s, "{") {
		decoded, err := decodeJSONText(s)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse %q as a JSON object for %v: %v", s, target, err)
		}
		entries, ok := decoded.(map[string]interface{})
		if !ok {
			return nil, wrapErr(ew, nil, 400, "cannot parse %q as a JSON object for %v", s, target)
		}
		return convertMap(reflect.ValueOf(entries), target, ew)
	}

	// "key:val,key:val" format.
	if strings.TrimSpace(s) == "" {
		return reflect.MakeMap(target).Interface(), nil
	}
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
		m.SetMapIndex(valueForType(k, keyType), valueForType(v, valType))
	}
	return m.Interface(), nil
}

// ConvertValue coerces value (any) to the target reflect.Type
// It is used by Structure.set() to normalise values arriving from JSON
// env vars or command-line flags after they have already been parsed into a Go value
func ConvertValue(value interface{}, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	// A nil target means the destination has no known concrete type
	// (eg an interface{} field whose current value is an untyped nil.
	// There is nothing to convert to, so return the value unchanged
	// instead of calling reflect methods that panic on a nil Type
	if target == nil {
		return value, nil
	}

	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	srcType := reflect.TypeOf(value)

	// Fast path: identical types.
	if srcType == target {
		return value, nil
	}

	// A text target accepts a scalar as its literal text (a JSON number keeps
	// its exact spelling), so `region = 12` or `debug = true` in a
	// text-oriented file still loads into a string field. Integers are never
	// turned into runes: 65 becomes "65", not "A"
	if target.Kind() == reflect.String {
		if text, ok := scalarText(value); ok {
			return reflect.ValueOf(text).Convert(target).Interface(), nil
		}
	}

	// A json.Number (from a decoder with UseNumber) carries the exact literal.
	// Normalise it to a Go numeric value first so integers beyond 2^53 reach an
	// int64/uint64 target without passing through a lossy float64, and so the
	// value stored for an untyped destination is never a json.Number
	if n, ok := value.(json.Number); ok {
		normalised, err := normalizeJSONNumber(n)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot parse number %q: %v", string(n), err)
		}
		value = normalised
		srcType = reflect.TypeOf(value)
		if srcType == target {
			return value, nil
		}
	}

	if srcType.AssignableTo(target) {
		return value, nil
	}

	// target is empty interface, accept anything.
	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return value, nil
	}

	// String source: delegate to ConvertString which knows all the formats.
	// reflect.Value.String (rather than a type assertion) also covers named
	// string types such as `type Level string`.
	if srcType.Kind() == reflect.String {
		return ConvertString(reflect.ValueOf(value).String(), target, ew)
	}

	// Numeric to numeric. This covers a time.Duration target too (its kind is
	// int64), so a float source gets the same NaN, infinity, whole-number and
	// range checks as for any integer target instead of storing whatever
	// int64(f) happens to produce
	if isNumeric(srcType) && isNumeric(target) {
		return convertNumeric(reflect.ValueOf(value), target)
	}

	// Slice to slice (element-wise, e.g. []interface{} to []string from JSON).
	if srcType.Kind() == reflect.Slice && target.Kind() == reflect.Slice {
		return convertSlice(reflect.ValueOf(value), target, ew)
	}

	// Slice to array, or to a pointer to one, element-wise. reflect.Convert
	// accepts both pairings but panics when the slice is shorter than the
	// array, so the length is checked here and a mismatch is an error rather
	// than a truncated or zero-padded array
	if srcType.Kind() == reflect.Slice {
		switch {
		case target.Kind() == reflect.Array:
			return convertArray(reflect.ValueOf(value), target, ew)
		case target.Kind() == reflect.Pointer && target.Elem().Kind() == reflect.Array:
			arr, err := convertArray(reflect.ValueOf(value), target.Elem(), ew)
			if err != nil {
				return nil, err
			}
			ptr := reflect.New(target.Elem())
			ptr.Elem().Set(reflect.ValueOf(arr))
			return ptr.Interface(), nil
		}
	}

	// Map to map (element-wise, e.g. map[string]interface{} to map[string]string).
	if srcType.Kind() == reflect.Map && target.Kind() == reflect.Map {
		return convertMap(reflect.ValueOf(value), target, ew)
	}

	// Standard reflect conversion (handles same-kind numeric aliases, etc.),
	// limited to the pairings that are both safe and meaningful here; the
	// rest fall through to the (usually failing) JSON round-trip below
	if convertible(srcType, target) {
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

// scalarText renders a number or boolean as the text a string field should
// receive: the exact literal for a json.Number, strconv formatting otherwise.
// It reports false for anything that is not such a scalar
func scalarText(value interface{}) (string, bool) {
	if n, ok := value.(json.Number); ok {
		return string(n), true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true
	case reflect.Float32:
		return strconv.FormatFloat(v.Float(), 'g', -1, 32), true
	case reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64), true
	}
	return "", false
}

func isInteger(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// convertible reports whether reflect.Value.Convert from src to target is a
// conversion ConvertValue may hand to reflect. ConvertibleTo also admits the
// pairings that must never reach Convert: an integer to a string, which yields
// the rune with that code point ("A" for 65) rather than the digits, which is
// never what a configuration caller means; and a slice to an array or to an
// array pointer, the only conversions Go defines to panic at run time (when
// the slice is shorter than the array). Both are checked by kind here rather
// than recovered from
func convertible(src, target reflect.Type) bool {
	if !src.ConvertibleTo(target) {
		return false
	}
	if isInteger(src) && target.Kind() == reflect.String {
		return false
	}
	if src.Kind() == reflect.Slice {
		if target.Kind() == reflect.Array {
			return false
		}
		if target.Kind() == reflect.Pointer && target.Elem().Kind() == reflect.Array {
			return false
		}
	}
	return true
}

// normalizeJSONNumber converts a json.Number literal to the Go value cfggo
// stores for an untyped destination. Integers that float64 can represent
// exactly (|n| <= 2^53) become float64, matching what encoding/json produces
// without UseNumber, so the dynamic type seen by callers is unchanged for
// ordinary values. Larger integers become int64 (or uint64 above MaxInt64) so
// no precision is lost; everything else is a float64
func normalizeJSONNumber(n json.Number) (interface{}, error) {
	s := string(n)
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		if f := float64(i); int64(f) == i && f >= -(1<<53) && f <= 1<<53 {
			return f, nil
		}
		return i, nil
	} else if errors.Is(err, strconv.ErrRange) {
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return u, nil
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// NormalizeJSONNumbers walks a value decoded by a json.Decoder with UseNumber
// and replaces every json.Number (including those nested in
// map[string]interface{} and []interface{} containers) with the Go numeric
// value described by normalizeJSONNumber. Containers are rebuilt, so the input
// is not modified. It is used by the JSON load path so untyped configuration
// keys never hold a json.Number
func NormalizeJSONNumbers(v interface{}) (interface{}, error) {
	switch x := v.(type) {
	case json.Number:
		return normalizeJSONNumber(x)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, elem := range x {
			n, err := NormalizeJSONNumbers(elem)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			out[k] = n
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, elem := range x {
			n, err := NormalizeJSONNumbers(elem)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			out[i] = n
		}
		return out, nil
	default:
		return v, nil
	}
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
		var v int64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v = src.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			u := src.Uint()
			if u > uint64(maxSigned(target.Bits())) {
				return nil, fmt.Errorf("numeric overflow: %d cannot fit in %v", u, target)
			}
			v = int64(u)
		case reflect.Float32, reflect.Float64:
			f := src.Float()
			if !isWholeFinite(f) || f < float64(minSigned(target.Bits())) || f > float64(maxSigned(target.Bits())) {
				return nil, fmt.Errorf("lossy numeric conversion: %v cannot fit exactly in %v", f, target)
			}
			v = int64(f)
			// Round-trip check: Go's spec says float-to-int overflow is
			// implementation-defined behavior. A value that passes the bounds
			// check on one platform may not round-trip on another (e.g. wasm,
			// 32-bit architectures, or future Go versions). Verify the
			// conversion is exact by round-tripping through the integer type.
			if float64(v) != f {
				return nil, fmt.Errorf("lossy numeric conversion: %v cannot fit exactly in %v", f, target)
			}
		}
		if out.OverflowInt(v) {
			return nil, fmt.Errorf("numeric overflow: %d cannot fit in %v", v, target)
		}
		out.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var v uint64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			i := src.Int()
			if i < 0 {
				return nil, fmt.Errorf("numeric underflow: %d cannot fit in %v", i, target)
			}
			v = uint64(i)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v = src.Uint()
		case reflect.Float32, reflect.Float64:
			f := src.Float()
			if !isWholeFinite(f) || f < 0 || f > float64(maxUnsigned(target.Bits())) {
				return nil, fmt.Errorf("lossy numeric conversion: %v cannot fit exactly in %v", f, target)
			}
			v = uint64(f)
			if float64(v) != f {
				return nil, fmt.Errorf("lossy numeric conversion: %v cannot fit exactly in %v", f, target)
			}
		}
		if out.OverflowUint(v) {
			return nil, fmt.Errorf("numeric overflow: %d cannot fit in %v", v, target)
		}
		out.SetUint(v)
	case reflect.Float32, reflect.Float64:
		var v float64
		switch src.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v = float64(src.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			v = float64(src.Uint())
		case reflect.Float32, reflect.Float64:
			v = src.Float()
		}
		if out.OverflowFloat(v) {
			return nil, fmt.Errorf("numeric overflow: %v cannot fit in %v", v, target)
		}
		out.SetFloat(v)
	}
	return out.Interface(), nil
}

func isWholeFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && math.Trunc(f) == f
}

func minSigned(bits int) int64 {
	if bits >= 64 {
		return -1 << 63
	}
	return -1 << (bits - 1)
}

func maxSigned(bits int) int64 {
	if bits >= 64 {
		return 1<<63 - 1
	}
	return 1<<(bits-1) - 1
}

func maxUnsigned(bits int) uint64 {
	if bits >= 64 {
		return ^uint64(0)
	}
	return 1<<bits - 1
}

func convertSlice(src reflect.Value, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	elemType := target.Elem()
	out := reflect.MakeSlice(target, src.Len(), src.Len())
	for i := 0; i < src.Len(); i++ {
		elem, err := ConvertValue(src.Index(i).Interface(), elemType, ew)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot convert slice[%d]: %v", i, err)
		}
		out.Index(i).Set(valueForType(elem, elemType))
	}
	return out.Interface(), nil
}

// convertArray fills a fresh array of type target from the elements of the
// slice src, converting each to the array's element type. The lengths must
// match exactly: a shorter slice would leave zero-valued elements and a longer
// one would silently drop values
func convertArray(src reflect.Value, target reflect.Type, ew ErrorWrapper) (interface{}, error) {
	if src.Len() != target.Len() {
		return nil, wrapErr(ew, nil, 400, "cannot convert %v of length %d to %v: length must be %d", src.Type(), src.Len(), target, target.Len())
	}
	elemType := target.Elem()
	out := reflect.New(target).Elem()
	for i := 0; i < src.Len(); i++ {
		elem, err := ConvertValue(src.Index(i).Interface(), elemType, ew)
		if err != nil {
			return nil, wrapErr(ew, err, 400, "cannot convert array[%d]: %v", i, err)
		}
		out.Index(i).Set(valueForType(elem, elemType))
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
		out.SetMapIndex(valueForType(ck, target.Key()), valueForType(cv, target.Elem()))
	}
	return out.Interface(), nil
}

func valueForType(v interface{}, target reflect.Type) reflect.Value {
	if v == nil {
		return reflect.Zero(target)
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(target) {
		return rv
	}
	if rv.Type().ConvertibleTo(target) {
		return rv.Convert(target)
	}
	return reflect.Zero(target)
}

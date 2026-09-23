package validcfg

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"unicode/utf8"
)

// Common validators

// Required returns a validator that checks if a value is not nil. A typed nil
// pointer, func, chan, map or slice (e.g. (*T)(nil)) counts as nil, and empty
// strings, slices and maps are rejected too
func Required() Validator {
	return func(value interface{}) error {
		if value == nil {
			return errors.New("value is required")
		}

		// Check for empty strings
		if s, ok := value.(string); ok && s == "" {
			return errors.New("value is required")
		}

		v := reflect.ValueOf(value)

		// A typed nil is not == nil once stored in an interface{}, so ask reflect
		switch v.Kind() {
		case reflect.Ptr, reflect.Func, reflect.Chan, reflect.Interface, reflect.Map, reflect.Slice:
			if v.IsNil() {
				return errors.New("value is required")
			}
		}

		// Check for zero values in slices and maps
		if (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0 {
			return errors.New("value is required")
		}

		return nil
	}
}

// MinLength returns a validator that checks if a string has at least min
// characters (Unicode code points, not bytes) or a slice, map or array has at
// least min elements
func MinLength(min int) Validator {
	return func(value interface{}) error {
		if value == nil {
			return errors.New("value is required")
		}
		v := reflect.ValueOf(value)

		switch v.Kind() {
		case reflect.String:
			if utf8.RuneCountInString(v.String()) < min {
				return fmt.Errorf("length must be at least %d characters", min)
			}
		case reflect.Slice, reflect.Map, reflect.Array:
			if v.Len() < min {
				return fmt.Errorf("must contain at least %d elements", min)
			}
		default:
			return fmt.Errorf("MinLength validator can only be applied to strings, slices, maps, or arrays, got %s", v.Kind())
		}

		return nil
	}
}

// MaxLength returns a validator that checks if a string has at most max
// characters (Unicode code points, not bytes) or a slice, map or array has at
// most max elements
func MaxLength(max int) Validator {
	return func(value interface{}) error {
		if value == nil {
			return errors.New("value is required")
		}
		v := reflect.ValueOf(value)

		switch v.Kind() {
		case reflect.String:
			if utf8.RuneCountInString(v.String()) > max {
				return fmt.Errorf("length must be at most %d characters", max)
			}
		case reflect.Slice, reflect.Map, reflect.Array:
			if v.Len() > max {
				return fmt.Errorf("must contain at most %d elements", max)
			}
		default:
			return fmt.Errorf("MaxLength validator can only be applied to strings, slices, maps, or arrays, got %s", v.Kind())
		}

		return nil
	}
}

// Range returns a validator that checks if a number is within a range
func Range(min, max float64) Validator {
	return func(value interface{}) error {
		if value == nil {
			return errors.New("value is required")
		}
		var val float64

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val = float64(v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val = float64(v.Uint())
		case reflect.Float32, reflect.Float64:
			val = v.Float()
		default:
			return fmt.Errorf("Range validator can only be applied to numeric types, got %s", v.Kind())
		}

		// NaN compares false against everything, so it would otherwise pass
		// any range
		if math.IsNaN(val) {
			return fmt.Errorf("value must be between %v and %v, got NaN", min, max)
		}

		if val < min || val > max {
			return fmt.Errorf("value must be between %v and %v", min, max)
		}

		return nil
	}
}

// OneOf returns a validator that checks if a value is one of the provided
// options. Numeric values are compared by value rather than by Go type, so
// OneOf(1, 2, 3) accepts int64(2), uint16(2) and 2.0 (cfggo stores values
// converted to the field's declared type, which rarely matches an untyped
// constant). The comparison is exact: 2.5 never matches 2, and integers are
// compared as integers rather than through a lossy float64. Non-numeric values
// must equal an option under reflect.DeepEqual
func OneOf(options ...interface{}) Validator {
	msg := fmt.Sprintf("value must be one of %v", options)
	return func(value interface{}) error {
		for _, option := range options {
			if reflect.DeepEqual(value, option) || numericEqual(value, option) {
				return nil
			}
		}

		return errors.New(msg)
	}
}

// numericClass groups the numeric reflect kinds so numericEqual can pick an
// exact comparison for each pairing. The order matters: numericEqual sorts its
// operands by class so it only handles each mixed pairing once
type numericClass int

const (
	notNumeric numericClass = iota
	signedInt
	unsignedInt
	floatNum
)

func numericClassOf(k reflect.Kind) numericClass {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return signedInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return unsignedInt
	case reflect.Float32, reflect.Float64:
		return floatNum
	}
	return notNumeric
}

// numericEqual reports whether a and b are both numbers with the same value,
// whatever their Go types. Integers are compared exactly (never via float64,
// which cannot represent every int64/uint64) and a float only equals an
// integer when it is a whole number the integer type can hold. NaN equals
// nothing. Non-numeric operands, including nil, never compare equal
func numericEqual(a, b interface{}) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	ca, cb := numericClassOf(va.Kind()), numericClassOf(vb.Kind())
	if ca == notNumeric || cb == notNumeric {
		return false
	}
	if ca > cb {
		va, vb, ca, cb = vb, va, cb, ca
	}

	switch {
	case ca == signedInt && cb == signedInt:
		return va.Int() == vb.Int()
	case ca == signedInt && cb == unsignedInt:
		return va.Int() >= 0 && uint64(va.Int()) == vb.Uint()
	case ca == unsignedInt && cb == unsignedInt:
		return va.Uint() == vb.Uint()
	case ca == signedInt && cb == floatNum:
		f := vb.Float()
		return isWholeFloat(f) && f >= math.MinInt64 && f < 1<<63 && int64(f) == va.Int()
	case ca == unsignedInt && cb == floatNum:
		f := vb.Float()
		return isWholeFloat(f) && f >= 0 && f < 1<<64 && uint64(f) == va.Uint()
	default: // both floats
		return va.Float() == vb.Float()
	}
}

// isWholeFloat reports whether f is a finite whole number (NaN and the
// infinities are not)
func isWholeFloat(f float64) bool {
	return !math.IsInf(f, 0) && f == math.Trunc(f)
}

// Regex returns a validator that checks if a string matches a regular
// expression. Like regexp.MustCompile it panics when pattern does not compile:
// patterns are expected to be program constants. To validate against a pattern
// that comes from input, compile it with regexp.Compile and wrap the check in
// Custom
func Regex(pattern string) Validator {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(fmt.Sprintf("invalid regex pattern: %s", err))
	}

	return func(value interface{}) error {
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("Regex validator can only be applied to strings, got %T", value)
		}

		if !re.MatchString(s) {
			return fmt.Errorf("value must match pattern: %s", pattern)
		}

		return nil
	}
}

var defaultEmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Email returns a validator that checks if a string is a valid email address
func Email() Validator {
	return func(value interface{}) error {
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("Email validator can only be applied to strings, got %T", value)
		}

		if !defaultEmailRegex.MatchString(s) {
			return fmt.Errorf("value must match pattern: %s", defaultEmailRegex.String())
		}

		return nil
	}
}

// URL returns a validator that checks if a string is a valid URL
func URL() Validator {
	return func(value interface{}) error {
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("URL validator can only be applied to strings, got %T", value)
		}

		u, err := url.Parse(s)
		if err != nil {
			return fmt.Errorf("invalid URL: %v", err)
		}

		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("URL must have a scheme and host")
		}

		return nil
	}
}

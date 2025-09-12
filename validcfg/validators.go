package validcfg

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
)

// Common validators

// Required returns a validator that checks if a value is not nil
func Required() Validator {
	return func(value interface{}) error {
		if value == nil {
			return errors.New("value is required")
		}

		// Check for empty strings
		if s, ok := value.(string); ok && s == "" {
			return errors.New("value is required")
		}

		// Check for zero values in slices and maps
		v := reflect.ValueOf(value)
		if (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0 {
			return errors.New("value is required")
		}

		return nil
	}
}

// MinLength returns a validator that checks if a string or slice has at least min elements
func MinLength(min int) Validator {
	return func(value interface{}) error {
		v := reflect.ValueOf(value)

		switch v.Kind() {
		case reflect.String:
			if v.Len() < min {
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

// MaxLength returns a validator that checks if a string or slice has at most max elements
func MaxLength(max int) Validator {
	return func(value interface{}) error {
		v := reflect.ValueOf(value)

		switch v.Kind() {
		case reflect.String:
			if v.Len() > max {
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

		if val < min || val > max {
			return fmt.Errorf("value must be between %v and %v", min, max)
		}

		return nil
	}
}

// OneOf returns a validator that checks if a value is one of the provided options
func OneOf(options ...interface{}) Validator {
	return func(value interface{}) error {
		for _, option := range options {
			if reflect.DeepEqual(value, option) {
				return nil
			}
		}

		return fmt.Errorf("value must be one of %v", options)
	}
}

// Regex returns a validator that checks if a string matches a regular expression
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

// Email returns a validator that checks if a string is a valid email address
func Email() Validator {
	emailRegex := `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`
	return Regex(emailRegex)
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

package cfggo

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
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

// validationMap stores validation rules for configuration keys
var validationMap = make(map[string]map[string]Validator)
var validationMutex sync.RWMutex

// RegisterValidator registers a validation function for a configuration key
func (c *Structure) RegisterValidator(key string, validator Validator) {
	if c.parent == nil {
		c.InitSelf()
	}

	validationMutex.Lock()
	defer validationMutex.Unlock()

	// Initialize the map for this config if it doesn't exist
	if _, exists := validationMap[c.name]; !exists {
		validationMap[c.name] = make(map[string]Validator)
	}

	validationMap[c.name][key] = validator
}

// Validate validates all configuration values against registered validators
func (c *Structure) Validate() error {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	validationMutex.RLock()
	defer configMutex.RUnlock()
	defer validationMutex.RUnlock()

	var errors ValidationErrors

	// Get validators for this config
	validators, exists := validationMap[c.name]
	if !exists {
		return nil // No validators registered for this config
	}

	for key, value := range c.configData {
		if validator, exists := validators[key]; exists {
			if err := validator(value); err != nil {
				errors = append(errors, ValidationError{
					Key: key,
					Err: err,
				})
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// ValidateKey validates a specific configuration key against its registered validator
func (c *Structure) ValidateKey(key string) error {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	validationMutex.RLock()
	defer configMutex.RUnlock()
	defer validationMutex.RUnlock()

	// Get validators for this config
	validators, exists := validationMap[c.name]
	if !exists {
		return nil // No validators registered for this config
	}

	value, exists := c.configData[key]
	if !exists {
		return c.WrapError(nil, 404, "Configuration key %s not found", key)
	}

	validator, exists := validators[key]
	if !exists {
		return nil // No validator registered for this key
	}

	if err := validator(value); err != nil {
		return c.WrapError(err, 0, "Validation failed for key %s", key)
	}

	return nil
}

// AddValidator adds a validator for a configuration key
func (c *Structure) AddValidator(key string, validator Validator) {
	validationMutex.Lock()
	defer validationMutex.Unlock()

	// Initialize the map for this config if it doesn't exist
	if _, exists := validationMap[c.name]; !exists {
		validationMap[c.name] = make(map[string]Validator)
	}

	validationMap[c.name][key] = validator
}

// Validate validates the configuration
func Validate(configName string, config map[string]interface{}) error {
	validationMutex.RLock()
	defer validationMutex.RUnlock()

	var errors ValidationErrors

	// Get validators for this config
	validators, exists := validationMap[configName]
	if !exists {
		return nil
	}

	// Validate each key
	for key, value := range config {
		if validator, exists := validators[key]; exists {
			if err := validator(value); err != nil {
				errors = append(errors, ValidationError{
					Key: key,
					Err: err,
				})
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

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

// Custom returns a validator that uses a custom function
func Custom(fn func(interface{}) error) Validator {
	return fn
}

// All returns a validator that checks if all validators pass
func All(validators ...Validator) Validator {
	return func(value interface{}) error {
		for _, validator := range validators {
			if err := validator(value); err != nil {
				return err
			}
		}

		return nil
	}
}

// Any returns a validator that checks if any validator passes
func Any(validators ...Validator) Validator {
	return func(value interface{}) error {
		var errors []error

		for _, validator := range validators {
			if err := validator(value); err == nil {
				return nil
			} else {
				errors = append(errors, err)
			}
		}

		return fmt.Errorf("none of the validators passed: %v", errors)
	}
}

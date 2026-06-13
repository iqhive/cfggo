package cfggo

import (
	"github.com/iqhive/cfggo/validcfg"
)

// --- Validation methods on Structure -----------------------------------------

// RegisterValidator registers a validator for key on this configuration instance.
func (c *Structure) RegisterValidator(key string, validator validcfg.Validator) {
	c.ensureInit()

	c.validationMutex.Lock()
	defer c.validationMutex.Unlock()
	if c.validationMap == nil {
		c.validationMap = make(map[string]map[string]validcfg.Validator)
	}

	if _, exists := c.validationMap[c.name]; !exists {
		c.validationMap[c.name] = make(map[string]validcfg.Validator)
	}

	c.validationMap[c.name][key] = validator
}

// AddValidator is an alias for RegisterValidator.
func (c *Structure) AddValidator(key string, validator validcfg.Validator) {
	c.ensureInit()

	c.validationMutex.Lock()
	defer c.validationMutex.Unlock()

	if c.validationMap == nil {
		c.validationMap = make(map[string]map[string]validcfg.Validator)
	}
	if _, exists := c.validationMap[c.name]; !exists {
		c.validationMap[c.name] = make(map[string]validcfg.Validator)
	}

	c.validationMap[c.name][key] = validator
}

// Validate runs all registered validators for this configuration instance.
func (c *Structure) Validate() error {
	c.ensureInit()

	c.configMutex.RLock()
	c.validationMutex.RLock()
	defer c.configMutex.RUnlock()
	defer c.validationMutex.RUnlock()

	var errs validcfg.ValidationErrors

	validators, exists := c.validationMap[c.name]
	if !exists {
		return nil
	}

	for key, value := range c.configData {
		if validator, exists := validators[key]; exists {
			if err := validator(value); err != nil {
				ve := validcfg.ValidationError{Key: key, Err: err}
				errs = append(errs, ve.WithProvenance(value, c.provenance[key].String()))
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateKey runs the registered validator (if any) for a single key.
func (c *Structure) ValidateKey(key string) error {
	c.ensureInit()

	c.configMutex.RLock()
	c.validationMutex.RLock()
	defer c.configMutex.RUnlock()
	defer c.validationMutex.RUnlock()

	value, exists := c.configData[key]
	if !exists {
		return c.WrapError(ErrUnknownKey, ErrCodeNotFound, "key %q not found", key)
	}

	validators, exists := c.validationMap[c.name]
	if !exists {
		return nil
	}

	validator, exists := validators[key]
	if !exists {
		return nil
	}

	if err := validator(value); err != nil {
		// Wrap as a ValidationError so the result matches both ErrValidation
		// (via errors.Is) and the underlying validator error. Provenance is
		// attached so the message points at the source of the bad value.
		ve := validcfg.ValidationError{Key: key, Err: err}.WithProvenance(value, c.provenance[key].String())
		return c.WrapError(ve, ErrCodeNone, "")
	}

	return nil
}

// --- Root-level validator constructors (re-exported from validcfg) ------------

// Required ensures a value is not nil or empty.
func Required() validcfg.Validator { return validcfg.Required() }

// MinLength checks that a string or slice has at least min elements.
func MinLength(min int) validcfg.Validator { return validcfg.MinLength(min) }

// MaxLength checks that a string or slice has at most max elements.
func MaxLength(max int) validcfg.Validator { return validcfg.MaxLength(max) }

// Range checks that a numeric value is within [min, max].
func Range(min, max float64) validcfg.Validator { return validcfg.Range(min, max) }

// OneOf checks that a value equals one of the provided options.
func OneOf(options ...interface{}) validcfg.Validator { return validcfg.OneOf(options...) }

// Regex validates a string against a regular expression pattern.
func Regex(pattern string) validcfg.Validator { return validcfg.Regex(pattern) }

// Email validates that a string is a valid email address.
func Email() validcfg.Validator { return validcfg.Email() }

// URL validates that a string is a valid URL with a scheme and host.
func URL() validcfg.Validator { return validcfg.URL() }

// All requires all of the provided validators to pass.
func All(validators ...validcfg.Validator) validcfg.Validator { return validcfg.All(validators...) }

// Any requires at least one of the provided validators to pass.
func Any(validators ...validcfg.Validator) validcfg.Validator { return validcfg.Any(validators...) }

// Custom wraps a plain function as a Validator.
func Custom(fn func(interface{}) error) validcfg.Validator { return validcfg.Custom(fn) }

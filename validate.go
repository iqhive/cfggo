package cfggo

import (
	"fmt"
	"sort"

	"github.com/iqhive/cfggo/validcfg"
)

// --- Validation methods on Structure -----------------------------------------

// RegisterValidator registers a validator for key on this configuration instance.
func (c *Structure) RegisterValidator(key string, validator validcfg.Validator) {
	c.validationMutex.Lock()
	defer c.validationMutex.Unlock()
	if c.validationMap == nil {
		c.validationMap = make(map[string]validcfg.Validator)
	}
	c.validationMap[key] = validator
}

// AddValidator is an alias for RegisterValidator.
func (c *Structure) AddValidator(key string, validator validcfg.Validator) {
	c.RegisterValidator(key, validator)
}

// ValidateConfigShape verifies that configuration metadata registered by code
// matches the struct shape. It currently checks that every registered validator
// targets a known config key, catching typos such as WithValidation("prot", ...)
// during Init instead of silently leaving the validator unused.
func (c *Structure) ValidateConfigShape() error {
	c.ensureInit()
	return c.validateConfigShape()
}

func (c *Structure) validateConfigShape() error {
	if c.plan == nil {
		return nil
	}

	c.validationMutex.RLock()
	unknown := make([]string, 0)
	for key := range c.validationMap {
		if _, ok := c.plan.byKey[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	c.validationMutex.RUnlock()

	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return c.WrapError(
		wrapKind(ErrUnknownKey, fmt.Errorf("validators registered for unknown configuration keys: %s", c.formatUnknownKeys(unknown))),
		ErrCodeNotFound,
		"invalid configuration shape",
	)
}

// Validate runs all registered validators for this configuration instance.
func (c *Structure) Validate() error {
	c.ensureInit()
	return c.validate()
}

func (c *Structure) validate() error {
	c.configMutex.RLock()
	c.validationMutex.RLock()
	defer c.configMutex.RUnlock()
	defer c.validationMutex.RUnlock()

	if len(c.validationMap) == 0 {
		return nil
	}

	var errs validcfg.ValidationErrors
	for key, value := range c.configData {
		if validator, exists := c.validationMap[key]; exists {
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
		return c.WrapError(ErrUnknownKey, ErrCodeNotFound, "key %q not found%s", key, c.didYouMeanSuffix(key))
	}

	validator, exists := c.validationMap[key]
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

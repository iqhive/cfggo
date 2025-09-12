package cfggo

import (
	"github.com/iqhive/cfggo/validcfg"
)

// RegisterValidator registers a validation function for a configuration key
func (c *Structure) RegisterValidator(key string, validator validcfg.Validator) {
	if c.parent == nil {
		c.InitSelf()
	}

	c.validationMutex.Lock()
	defer c.validationMutex.Unlock()
	if c.validationMap == nil {
		c.validationMap = make(map[string]map[string]validcfg.Validator)
	}

	// Initialize the map for this config if it doesn't exist
	if _, exists := c.validationMap[c.name]; !exists {
		c.validationMap[c.name] = make(map[string]validcfg.Validator)
	}

	c.validationMap[c.name][key] = validator
}

// Validate validates all configuration values against registered validators
func (c *Structure) Validate() error {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	c.validationMutex.RLock()
	defer configMutex.RUnlock()
	defer c.validationMutex.RUnlock()

	var errors validcfg.ValidationErrors

	// Get validators for this config
	validators, exists := c.validationMap[c.name]
	if !exists {
		return nil // No validators registered for this config
	}

	for key, value := range c.configData {
		if validator, exists := validators[key]; exists {
			if err := validator(value); err != nil {
				errors = append(errors, validcfg.ValidationError{
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
	c.validationMutex.RLock()
	defer configMutex.RUnlock()
	defer c.validationMutex.RUnlock()

	// Get validators for this config
	validators, exists := c.validationMap[c.name]
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
func (c *Structure) AddValidator(key string, validator validcfg.Validator) {
	c.validationMutex.Lock()
	defer c.validationMutex.Unlock()

	// Initialize the map for this config if it doesn't exist
	if _, exists := c.validationMap[c.name]; !exists {
		c.validationMap[c.name] = make(map[string]validcfg.Validator)
	}

	c.validationMap[c.name][key] = validator
}

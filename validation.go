package cfggo

import (
	"sync"
)

// Validator is a function that validates a configuration value
type Validator func(interface{}) error

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

	var errors []error

	// Get validators for this config
	validators, exists := validationMap[c.name]
	if !exists {
		return nil // No validators registered for this config
	}

	for key, value := range c.configData {
		if validator, exists := validators[key]; exists {
			if err := validator(value); err != nil {
				errors = append(errors, ErrorWrapper(err, 0, "Validation failed for key %s", key))
			}
		}
	}

	if len(errors) > 0 {
		var errMsg string
		for _, err := range errors {
			errMsg += err.Error() + "\n"
		}
		return ErrorWrapper(nil, 400, "Configuration validation failed:\n%s", errMsg)
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
		return ErrorWrapper(nil, 404, "Configuration key %s not found", key)
	}

	validator, exists := validators[key]
	if !exists {
		return nil // No validator registered for this key
	}

	if err := validator(value); err != nil {
		return ErrorWrapper(err, 0, "Validation failed for key %s", key)
	}

	return nil
}

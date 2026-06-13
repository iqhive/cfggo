package cfggo

import (
	"reflect"
)

func (c *Structure) loadFromEnv() {
	if c.skipEnv {
		c.log().Debug("loadFromEnv: skipping environment variables")
		return
	}

	// Get all configuration keys
	var keys []string
	c.configMutex.RLock()
	for key := range c.configData {
		keys = append(keys, key)
	}
	c.configMutex.RUnlock()

	// Use internal env loader to get environment variables
	envVars := internalEnvLoader.LoadEnvironmentVariables(keys)

	// Process each environment variable found
	for key, value := range envVars {
		// Get the target type for this key
		c.configMutex.RLock()
		existing, exists := c.configData[key]
		c.configMutex.RUnlock()
		
		if !exists {
			continue
		}

		// Create a dynamic var to handle the conversion. applyLoaded (called by
		// dv.Set) records SourceEnv provenance and marks the config changed.
		dv := &dynamicVar{
			config: c,
			name:   key,
			want:   reflect.TypeOf(existing),
			source: SourceEnv,
		}

		// Set the value
		if err := dv.Set(value); err != nil {
			envVar := internalEnvLoader.KeyToEnvVar(key)
			c.logInfof("Error setting config from environment variable %s=(%v): %v", envVar, value, err)
		} else {
			envVar := internalEnvLoader.KeyToEnvVar(key)
			c.logDebugf("Set config %s from environment variable %s=(%v)", key, envVar, value)
		}
	}
}
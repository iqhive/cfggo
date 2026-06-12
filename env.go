package cfggo

import (
	"reflect"
)

func (c *Structure) loadFromEnv() {
	if c.skipEnv {
		Logger.Debug("loadFromEnv: skipping environment variables")
		return
	}

	// Get all configuration keys
	var keys []string
	configMutex.RLock()
	for key := range c.configData {
		keys = append(keys, key)
	}
	configMutex.RUnlock()

	// Use internal env loader to get environment variables
	envVars := internalEnvLoader.LoadEnvironmentVariables(keys)

	// Process each environment variable found
	for key, value := range envVars {
		// Get the target type for this key
		configMutex.RLock()
		existing, exists := c.configData[key]
		configMutex.RUnlock()
		
		if !exists {
			continue
		}

		// Create a dynamic var to handle the conversion
		dv := &dynamicVar{
			config: c,
			name:   key,
			want:   reflect.TypeOf(existing),
		}

		// Set the value
		if err := dv.Set(value); err != nil {
			envVar := internalEnvLoader.KeyToEnvVar(key)
			Logger.Infof("Error setting config from environment variable %s=(%v): %v", envVar, value, err)
		} else {
			envVar := internalEnvLoader.KeyToEnvVar(key)
			Logger.Debugf("Set config %s from environment variable %s=(%v)", key, envVar, value)
			
			// Mark that configuration has changed
			configMutex.Lock()
			c.changed = true
			configMutex.Unlock()
		}
	}
}
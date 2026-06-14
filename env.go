package cfggo

import (
	"os"
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

	// Use internal env loader to get environment variables.
	envVars := make(map[string]string)
	for _, key := range keys {
		envVar := c.envVarName(key)
		if value, exists := os.LookupEnv(envVar); exists {
			envVars[key] = value
		}
	}

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
		envVar := c.envVarName(key)
		if err := dv.Set(value); err != nil {
			c.log().Info("cfggo: error setting config from environment variable", "key", key, "env", envVar, "value", value, "err", err)
		} else {
			c.log().Debug("cfggo: set config from environment variable", "key", key, "env", envVar, "value", value)
		}
	}
}

func (c *Structure) envVarName(key string) string {
	envVar := internalEnvLoader.KeyToEnvVar(key)
	if c.envPrefix == "" {
		return envVar
	}
	return c.envPrefix + envVar
}

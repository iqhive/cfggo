package cfggo

import (
	"errors"
	"fmt"
	"os"
	"reflect"
)

func (c *Structure) loadFromEnv() error {
	if c.skipEnv {
		c.log().Debug("loadFromEnv: skipping environment variables")
		return nil
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
	var setErrs []error
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

		// Set the value. Secret-tagged fields are redacted from log output and
		// from the returned error (conversion errors quote the raw input), so a
		// credential supplied via the environment never reaches the logs
		envVar := c.envVarName(key)
		secret := c.isSecretKey(key)
		loggedValue := value
		if secret {
			loggedValue = maskedValue
		}
		if err := dv.Set(value); err != nil {
			if secret {
				err = fmt.Errorf("invalid value (redacted: field is secret)")
			}
			c.log().Info("cfggo: error setting config from environment variable", "key", key, "env", envVar, "value", loggedValue, "err", err)
			setErrs = append(setErrs, fmt.Errorf("key %q from env %s: %w", key, envVar, err))
		} else {
			c.log().Debug("cfggo: set config from environment variable", "key", key, "env", envVar, "value", loggedValue)
		}
	}
	if len(setErrs) > 0 {
		return c.WrapError(wrapKind(ErrSource, errors.Join(setErrs...)), ErrCodeInvalidArgument, "failed to apply environment variables")
	}
	return nil
}

func (c *Structure) envVarName(key string) string {
	envVar := internalEnvLoader.KeyToEnvVar(key)
	if c.envPrefix == "" {
		return envVar
	}
	return c.envPrefix + envVar
}

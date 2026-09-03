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

	var setErrs []error
	for _, key := range c.getAllKeys() {
		if err := c.applyEnvForKey(key); err != nil {
			setErrs = append(setErrs, err)
		}
	}
	if len(setErrs) > 0 {
		return c.WrapError(wrapKind(ErrSource, errors.Join(setErrs...)), ErrCodeInvalidArgument, "failed to apply environment variables")
	}
	return nil
}

// applyEnvForKey reads the environment variable for key and applies it,
// recording SourceEnv provenance. It returns nil when the variable is unset
// or the key is unknown
func (c *Structure) applyEnvForKey(key string) error {
	envVar := c.envVarName(key)
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return nil
	}
	c.configMutex.RLock()
	existing, known := c.configData[key]
	c.configMutex.RUnlock()
	if !known {
		return nil
	}

	// An untyped nil value (eg a func() interface{} field with no default, or
	// a NewFlag key with a nil default) has no runtime type, so fall back to
	// the type declared for the key, and finally to interface{} so the text is
	// inferred rather than rejected
	want := reflect.TypeOf(existing)
	if want == nil {
		want = c.declaredType(key)
	}
	if want == nil {
		want = emptyInterfaceType
	}
	dv := &dynamicVar{config: c, name: key, want: want, source: SourceEnv}

	// Secret-tagged fields are redacted from log output and from the returned
	// error (conversion errors quote the raw input), so a credential supplied
	// via the environment never reaches the logs
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
		return fmt.Errorf("key %q from env %s: %w", key, envVar, err)
	}
	c.log().Debug("cfggo: set config from environment variable", "key", key, "env", envVar, "value", loggedValue)
	return nil
}

func (c *Structure) envVarName(key string) string {
	envVar := internalEnvLoader.KeyToEnvVar(key)
	if c.envPrefix == "" {
		return envVar
	}
	return c.envPrefix + envVar
}

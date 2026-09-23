package cfggo

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

func (c *Structure) loadFromEnv() error {
	if c.skipEnv {
		c.log().Debug("loadFromEnv: skipping environment variables")
		return nil
	}
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	return c.loadFromEnvLocked()
}

// loadFromEnvLocked applies the environment variable of every known key. The
// caller must hold configMutex for writing: Reload runs it while building its
// private candidate so no reader can observe a half-applied environment layer
func (c *Structure) loadFromEnvLocked() error {
	if c.skipEnv {
		c.log().Debug("loadFromEnv: skipping environment variables")
		return nil
	}

	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}

	var setErrs []error
	for _, key := range keys {
		if err := c.applyEnvForKeyLocked(key); err != nil {
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
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	return c.applyEnvForKeyLocked(key)
}

// applyEnvForKeyLocked is applyEnvForKey for callers that already hold
// configMutex for writing
func (c *Structure) applyEnvForKeyLocked(key string) error {
	envVar := c.envVarName(key)
	value, exists := os.LookupEnv(envVar)
	if !exists {
		return nil
	}
	existing, known := c.configData[key]
	if !known {
		return nil
	}

	// Convert into the type declared for the key (or the current value's type
	// for a NewFlag key), and finally to interface{} so the text of a key
	// without any known type is inferred rather than rejected
	want := c.storeTypeLocked(key, existing)
	if want == nil {
		want = emptyInterfaceType
	}

	// Secret-tagged fields are redacted from log output and from the returned
	// error (conversion errors quote the raw input), so a credential supplied
	// via the environment never reaches the logs
	secret := c.isSecretKey(key)
	loggedValue := value
	if secret {
		loggedValue = maskedValue
	}
	if err := c.applyEnvValueLocked(key, value, want); err != nil {
		c.log().Info("cfggo: error setting config from environment variable", "key", key, "env", envVar, "value", loggedValue, "err", err)
		return fmt.Errorf("key %q from env %s: %w", key, envVar, err)
	}
	c.log().Debug("cfggo: set config from environment variable", "key", key, "env", envVar, "value", loggedValue)
	return nil
}

// applyEnvValueLocked converts the environment text to want and stores it with
// SourceEnv provenance. The caller must hold configMutex for writing
func (c *Structure) applyEnvValueLocked(key, text string, want reflect.Type) error {
	value, err := iconvert.ConvertString(text, want, c.convertWrapper(key))
	if err != nil {
		return c.WrapError(c.redactSecretValueError(key, err), ErrCodeInvalidArgument, "key %q from %s", key, SourceEnv)
	}
	if err := c.set(key, value); err != nil {
		return c.WrapError(c.redactSecretValueError(key, err), ErrCodeInvalidArgument, "key %q from %s", key, SourceEnv)
	}
	c.recordSourceLocked(key, SourceEnv)
	return nil
}

func (c *Structure) envVarName(key string) string {
	envVar := internalEnvLoader.KeyToEnvVar(key)
	if c.envPrefix == "" {
		return envVar
	}
	return c.envPrefix + envVar
}

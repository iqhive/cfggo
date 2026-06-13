package cfggo

import (
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

// Set sets a configuration value and propagates it to the config struct.
// On success it records the value's provenance as SourceSet and fires any
// OnChange callbacks for the key.
func (c *Structure) Set(key string, value interface{}) error {
	c.ensureInit()

	c.configMutex.Lock()
	err := c.set(key, value)
	if err == nil {
		c.changed = true
		c.recordSourceLocked(key, SourceSet)
	}
	c.configMutex.Unlock()

	if err != nil {
		return err
	}
	c.notifyChange([]string{key})
	return nil
}

// applyLoaded stores key=value originating from a loading layer (a default
// tag, an environment variable, ...). It takes the write lock, records
// provenance, and marks the config changed, but deliberately does NOT fire
// OnChange callbacks: those are reserved for Set and Reload.
func (c *Structure) applyLoaded(key string, value interface{}, src Source) error {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	if err := c.set(key, value); err != nil {
		return err
	}
	c.changed = true
	c.recordSourceLocked(key, src)
	return nil
}

// set is the internal, non-locking version of Set.
func (c *Structure) set(key string, value interface{}) error {
	if existing, exists := c.configData[key]; exists {
		existingType := reflect.TypeOf(existing)

		convertedValue, err := iconvert.ConvertValue(value, existingType, c)
		if err != nil {
			return err
		}

		c.configData[key] = convertedValue
		return nil
	}

	c.configData[key] = value
	return nil
}

// Get returns the current value for key and whether it was found.
func (c *Structure) Get(key string) (interface{}, bool) {
	c.ensureInit()

	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	value, exists := c.configData[key]
	return value, exists
}

func (c *Structure) getAllKeys() []string {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}
	return keys
}

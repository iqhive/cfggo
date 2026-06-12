package cfggo

import (
	"reflect"

	"github.com/iqhive/cfggo/convert"
)

// Set sets a configuration value and propagates it to the config struct.
func (c *Structure) Set(key string, value interface{}) error {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.Lock()
	defer configMutex.Unlock()
	c.changed = true
	return c.set(key, value)
}

// set is the internal, non-locking version of Set.
func (c *Structure) set(key string, value interface{}) error {
	if existing, exists := c.configData[key]; exists {
		existingType := reflect.TypeOf(existing)

		convertedValue, err := convert.ConvertValue(value, existingType, c)
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
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	value, exists := c.configData[key]
	return value, exists
}

func (c *Structure) getAllKeys() []string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}
	return keys
}

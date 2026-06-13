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
	if _, exists := c.configData[key]; !exists {
		c.configMutex.Unlock()
		return c.WrapError(ErrUnknownKey, ErrCodeNotFound, "Set: unknown configuration key %q", key)
	}
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
		if existingType == nil {
			// existing is an untyped nil, eg a func() interface{} field with no default
			// There is no concrete type to convert to, so store the incoming value as-is
			// rather than letting reflect panic on a nil Type
			c.configData[key] = value
			return nil
		}

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

// Value returns the current value for key typed as T, and whether a usable
// value was found. It is the type-safe alternative to Get for callers that know
// the expected type and want to avoid a manual interface{} assertion:
//
//	port, ok := cfggo.Value[int](&cfg.Structure, "port")
//
// ok is false when the key is absent or its stored value is neither a T nor
// convertible to one (in which case the zero value of T is returned). The fast
// path (the stored value is already a T) is a single map lookup and assertion
func Value[T any](c *Structure, key string) (T, bool) {
	var zero T
	raw, ok := c.Get(key)
	if !ok {
		return zero, false
	}
	if v, ok := raw.(T); ok {
		return v, true
	}
	if raw == nil {
		return zero, false
	}
	rv := reflect.ValueOf(raw)
	tt := reflect.TypeOf(&zero).Elem()
	if rv.Type().ConvertibleTo(tt) {
		if cv, ok := rv.Convert(tt).Interface().(T); ok {
			return cv, true
		}
	}
	return zero, false
}

// MustValue is like Value but returns only the value, falling back to the zero
// value of T when the key is missing or not convertible to T. Use it when a
// missing/!ok case is acceptable as the zero value; use Value when you need to
// distinguish "absent" from "present and zero"
func MustValue[T any](c *Structure, key string) T {
	v, _ := Value[T](c, key)
	return v
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

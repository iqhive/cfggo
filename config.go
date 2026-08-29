package cfggo

import (
	"reflect"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

// Set sets a configuration value and propagates it to the config struct.
// The value is checked against any validator registered for the key, so a
// runtime Set cannot bypass the validation enforced on the flag, file, and
// environment layers. On success it records the value's provenance as
// SourceSet and fires any OnChange callbacks for the key.
func (c *Structure) Set(key string, value interface{}) error {
	c.ensureInit()

	if err := c.validateForSet(key, value); err != nil {
		return err
	}

	// Exclude concurrent Reloads (which release configMutex between phases)
	// so this Set cannot be lost to a reload rollback or snapshot restore.
	// Released before callbacks fire so a callback may itself trigger a Reload
	c.reloadMutex.RLock()
	c.configMutex.Lock()
	old, exists := c.configData[key]
	if !exists {
		c.configMutex.Unlock()
		c.reloadMutex.RUnlock()
		return c.WrapError(ErrUnknownKey, ErrCodeNotFound, "Set: unknown configuration key %q%s", key, c.didYouMeanSuffix(key))
	}
	old = cloneMutableInterface(old)
	err := c.set(key, value)
	var newVal interface{}
	if err == nil {
		c.markChangedLocked()
		c.recordSourceLocked(key, SourceSet)
		newVal = cloneMutableInterface(c.configData[key])
	}
	c.configMutex.Unlock()
	c.reloadMutex.RUnlock()

	if err != nil {
		return c.WrapError(err, ErrCodeInvalidArgument, "Set: key %q from %s", key, SourceSet)
	}
	// Only assemble the change set when someone is listening,
	// so a plain Set stays allocation-free in the common case
	if c.hasListeners() {
		c.notifyChange([]Change{{Key: key, Old: old, New: newVal, Source: SourceSet}})
	}
	return nil
}

// validateForSet runs the registered validator (if any) for key against the
// value as it would be stored: converted to the key's current concrete type
// when one is known. It runs before any lock is taken because a validator is
// arbitrary user code that may itself read configuration.
func (c *Structure) validateForSet(key string, value interface{}) error {
	c.validationMutex.RLock()
	_, hasValidator := c.validationMap[key]
	c.validationMutex.RUnlock()
	if !hasValidator {
		return nil
	}

	c.configMutex.RLock()
	existing, exists := c.configData[key]
	c.configMutex.RUnlock()
	if !exists {
		return nil
	}

	checked := value
	if existingType := reflect.TypeOf(existing); existingType != nil {
		converted, err := iconvert.ConvertValue(value, existingType, c)
		if err != nil {
			return c.WrapError(err, ErrCodeInvalidArgument, "Set: key %q from %s", key, SourceSet)
		}
		checked = converted
	}
	return c.validateValueForKey(key, checked, SourceSet)
}

// applyLoaded stores key=value originating from a loading layer (a default
// tag, an environment variable, ...). It takes the write lock, records
// provenance, and marks the config changed, but deliberately does NOT fire
// OnChange callbacks: those are reserved for Set and Reload.
func (c *Structure) applyLoaded(key string, value interface{}, src Source) error {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	if err := c.set(key, value); err != nil {
		return c.WrapError(err, ErrCodeInvalidArgument, "key %q from %s", key, src)
	}
	c.markChangedLocked()
	c.recordSourceLocked(key, src)
	return nil
}

func (c *Structure) markChangedLocked() {
	c.changed = true
	c.changeVersion++
}

// set is the internal, non-locking version of Set.
func (c *Structure) set(key string, value interface{}) error {
	if existing, exists := c.configData[key]; exists {
		existingType := reflect.TypeOf(existing)
		if existingType == nil {
			// existing is an untyped nil, eg a func() interface{} field with no default
			// There is no concrete type to convert to, so store the incoming value as-is
			// rather than letting reflect panic on a nil Type
			c.configData[key] = cloneMutableInterface(value)
			return nil
		}

		convertedValue, err := iconvert.ConvertValue(value, existingType, c)
		if err != nil {
			return err
		}

		c.configData[key] = cloneMutableInterface(convertedValue)
		return nil
	}

	c.configData[key] = cloneMutableInterface(value)
	return nil
}

// Get returns the current value for key and whether it was found.
func (c *Structure) Get(key string) (interface{}, bool) {
	c.ensureInit()

	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	value, exists := c.configData[key]
	return cloneMutableInterface(value), exists
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
		if raw != nil {
			if cloned, ok := cloneMutableReflectValue(reflect.ValueOf(raw)).Interface().(T); ok {
				return cloned, true
			}
		}
		return v, true
	}
	if raw == nil {
		return zero, false
	}
	rv := reflect.ValueOf(raw)
	tt := reflect.TypeOf(&zero).Elem()
	if rv.Type().ConvertibleTo(tt) {
		cv := cloneMutableReflectValue(rv.Convert(tt))
		if cv, ok := cv.Interface().(T); ok {
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

// changedState returns whether the configuration has been modified and the
// current changeVersion, both observed under the read lock. It is used by
// cfggo.Group to decide when to persist the combined configuration.
func (c *Structure) changedState() (bool, uint64) {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	return c.changed, c.changeVersion
}

// clearChangedIf clears the changed flag only if changeVersion still matches
// the supplied value. This avoids losing a concurrent Set that incremented the
// version after the caller observed the changed state.
func (c *Structure) clearChangedIf(version uint64) {
	c.configMutex.Lock()
	if c.changeVersion == version {
		c.changed = false
	}
	c.configMutex.Unlock()
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

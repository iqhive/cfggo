package cfggo

// changeCallback pairs an OnChange listener with the id used to remove it.
type changeCallback struct {
	id int
	fn func(changedKeys []string)
}

// OnChange registers fn to be called whenever configuration values change after
// initialisation. fn receives the keys that changed.
//
// Callbacks fire on:
//   - Set: with the single key that was set.
//   - Reload / ReloadConfig: with the aggregate set of keys whose values
//     differ from before the reload (empty reloads do not fire).
//
// Callbacks do not fire for values applied during Init (defaults, the initial
// env/file/flag load). Callbacks run synchronously, after cfggo has released
// its internal lock, so it is safe to call Get/Set from within a callback (a
// Set from inside a callback will itself fire callbacks — avoid infinite
// loops). The returned function unregisters the callback; it is safe to call
// more than once.
func (c *Structure) OnChange(fn func(changedKeys []string)) (cancel func()) {
	if fn == nil {
		return func() {}
	}
	c.ensureInit()

	c.callbackMutex.Lock()
	c.nextCallbackID++
	id := c.nextCallbackID
	c.changeCallbacks = append(c.changeCallbacks, changeCallback{id: id, fn: fn})
	c.callbackMutex.Unlock()

	var once bool
	return func() {
		c.callbackMutex.Lock()
		defer c.callbackMutex.Unlock()
		if once {
			return
		}
		once = true
		for i, cb := range c.changeCallbacks {
			if cb.id == id {
				c.changeCallbacks = append(c.changeCallbacks[:i], c.changeCallbacks[i+1:]...)
				break
			}
		}
	}
}

// notifyChange invokes the registered callbacks with keys. It must be called
// without holding c.configMutex so callbacks may safely read or write config.
func (c *Structure) notifyChange(keys []string) {
	if len(keys) == 0 {
		return
	}
	c.callbackMutex.Lock()
	if len(c.changeCallbacks) == 0 {
		c.callbackMutex.Unlock()
		return
	}
	fns := make([]func([]string), len(c.changeCallbacks))
	for i, cb := range c.changeCallbacks {
		fns[i] = cb.fn
	}
	c.callbackMutex.Unlock()

	for _, fn := range fns {
		fn(keys)
	}
}

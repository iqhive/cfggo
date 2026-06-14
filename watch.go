package cfggo

// Change describes a single configuration value transition delivered to
// OnChange listeners. It carries enough context to audit or react to a change
// without a follow-up Get: the key, its previous and new values, and the source
// that produced the new value
type Change struct {
	// Key is the dotted configuration key that changed
	Key string
	// Old is the value before the change (nil if the key was previously unset)
	Old interface{}
	// New is the value after the change
	New interface{}
	// Source is where the new value came from (eg SourceSet, SourceFile)
	Source Source
}

// changeCallback pairs an OnChange listener with the id used to remove it.
type changeCallback struct {
	id int
	fn func(changes []Change)
}

// OnChange registers fn to be called whenever configuration values change after
// initialisation. fn receives one Change per affected key,
// including the old and new values and the source of the new value
//
// Callbacks fire on:
//   - Set: with a single Change for the key that was set (Source SourceSet).
//   - Reload / ReloadConfig: with one Change per key whose value differs from
//     before the reload (empty reloads do not fire)
//
// Callbacks do not fire for values applied during Init (defaults, the initial
// env/file/flag load). Callbacks run synchronously, after cfggo has released
// its internal lock, so it is safe to call Get/Set from within a callback (a
// Set from inside a callback will itself fire callbacks — avoid infinite
// loops). The returned function unregisters the callback; it is safe to call
// more than once.
func (c *Structure) OnChange(fn func(changes []Change)) (cancel func()) {
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

// hasListeners reports whether any OnChange callbacks are registered. Callers
// use it to skip building the (allocating) []Change slice when nobody is
// listening, keeping Set/Reload allocation-free in the common no-listener case
func (c *Structure) hasListeners() bool {
	c.callbackMutex.Lock()
	n := len(c.changeCallbacks)
	c.callbackMutex.Unlock()
	return n > 0
}

// notifyChange invokes the registered callbacks with changes. It must be called
// without holding c.configMutex so callbacks may safely read or write config.
func (c *Structure) notifyChange(changes []Change) {
	if len(changes) == 0 {
		return
	}
	c.callbackMutex.Lock()
	if len(c.changeCallbacks) == 0 {
		c.callbackMutex.Unlock()
		return
	}
	fns := make([]func([]Change), len(c.changeCallbacks))
	for i, cb := range c.changeCallbacks {
		fns[i] = cb.fn
	}
	c.callbackMutex.Unlock()

	for _, fn := range fns {
		fn(changes)
	}
}

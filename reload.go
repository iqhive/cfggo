package cfggo

import "reflect"

// Reload reloads the configuration from all sources
func (c *Structure) Reload() error {
	// First, make a copy of the current configuration for potential rollback
	var oldConfig map[string]interface{}
	// oldProvenance lets us re-assert command-line flag precedence after the
	// file/env layers below have been reloaded (see the restore step)
	var oldProvenance map[string]Source

	// Get a snapshot of the current configuration
	c.configMutex.Lock()
	oldConfig = make(map[string]interface{})
	for k, v := range c.configData {
		oldConfig[k] = v
	}
	oldProvenance = make(map[string]Source, len(c.provenance))
	for k, v := range c.provenance {
		oldProvenance[k] = v
	}
	// Reset the changed flag
	c.changed = false
	c.configMutex.Unlock()

	var err error

	// Attempt to reload configuration from file (loadConfig handles its own locking)
	if c.configHandler != nil {
		if err = c.loadConfig(false); err != nil {
			c.logErrorf("Failed to reload configuration from file: %v", err)

			// Rollback to old configuration on error
			c.configMutex.Lock()
			c.configData = oldConfig
			c.configMutex.Unlock()
			return err
		}
	}

	// Reload from environment variables
	c.loadFromEnv()

	// Check if flags have been parsed before calling parseFlags
	var flagsParsed bool
	c.configMutex.RLock()
	flagsParsed = c.FlagSet != nil && c.FlagSet.Parsed()
	c.configMutex.RUnlock()

	// Reload from flags if they've been parsed
	if flagsParsed {
		c.parseFlags()
	}

	// Re-assert command-line flag precedence. The standard flag package will
	// not re-run an already-parsed FlagSet (parseFlags above early-returns), so
	// the file and environment layers reloaded above can otherwise clobber a
	// value that the user supplied on the command line. Flags rank highest in
	// the documented precedence order, so restore any value whose pre-reload
	// provenance was SourceFlag.
	for key, src := range oldProvenance {
		if src != SourceFlag {
			continue
		}
		if v, ok := oldConfig[key]; ok {
			if err := c.applyLoaded(key, v, SourceFlag); err != nil {
				c.logWarnf("Reload: could not restore flag value for %s: %v", key, err)
			}
		}
	}

	// The accessor closures installed during Init read c.configData live on
	// every call, so the reloaded values are already visible without
	// reinstalling them. Re-running replaceConfigFuncs here would rewrite the
	// struct func fields, racing with any goroutine currently calling an
	// accessor (and with a concurrent reload)

	// Validate configuration after reloading
	if err = c.Validate(); err != nil {
		c.logWarnf("Configuration validation failed after reload: %v", err)
		// We don't return this error as it's just a warning
	}

	// Notify OnChange listeners with the aggregate set of keys whose values
	// differ from the pre-reload snapshot.
	c.notifyChange(c.changedKeys(oldConfig))

	return nil
}

// changedKeys compares the current configData against a previous snapshot and
// returns the keys whose values changed or were added.
func (c *Structure) changedKeys(old map[string]interface{}) []string {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()

	var keys []string
	for k, newVal := range c.configData {
		if oldVal, ok := old[k]; !ok || !reflect.DeepEqual(oldVal, newVal) {
			keys = append(keys, k)
		}
	}
	return keys
}

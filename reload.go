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
			c.log().Error("cfggo: failed to reload configuration source", "err", err)

			// Rollback to old configuration on error. Restore provenance too so
			// it stays consistent with the values after a failed reload. The
			// override-chain trail accumulated during the partial load is
			// dropped (SourceChain then falls back to the restored single
			// source); this keeps the success path free of trail-snapshot cost
			c.configMutex.Lock()
			c.configData = oldConfig
			c.provenance = oldProvenance
			c.provenanceTrail = nil
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
				c.log().Warn("cfggo: could not restore flag value during reload", "key", key, "err", err)
			}
		}
	}

	// The accessor closures installed during Init read c.configData live on
	// every call, so the reloaded values are already visible without
	// reinstalling them. Re-running replaceConfigFuncs here would rewrite the
	// struct func fields, racing with any goroutine currently calling an
	// accessor (and with a concurrent reload)

	// Validate configuration after reloading
	// This stays a warning (rather than an error) so a transient bad value
	// never tears down a running service mid-reload; the validation error now
	// carries each value's provenance so the log line points straight at the
	// offending source
	if err = c.Validate(); err != nil {
		c.log().Warn("cfggo: configuration validation failed after reload", "err", err)
	}

	// Notify OnChange listeners with the aggregate set of keys whose values
	// differ from the pre-reload snapshot
	// Skip the (allocating) diff entirely
	// when nobody is listening.
	if c.hasListeners() {
		c.notifyChange(c.changeSet(oldConfig))
	}

	return nil
}

// changeSet compares the current configData against a previous snapshot and
// returns one Change per key whose value changed or was added, annotated with
// the current source
func (c *Structure) changeSet(old map[string]interface{}) []Change {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()

	var changes []Change
	for k, newVal := range c.configData {
		if oldVal, ok := old[k]; !ok || !reflect.DeepEqual(oldVal, newVal) {
			changes = append(changes, Change{
				Key:    k,
				Old:    old[k],
				New:    newVal,
				Source: c.provenance[k],
			})
		}
	}
	return changes
}

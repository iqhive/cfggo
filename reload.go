package cfggo

import "reflect"

// Reload reloads the configuration from all sources
func (c *Structure) Reload() error {
	c.ensureInit()

	// First, make a copy of the current configuration for potential rollback
	var oldConfig map[string]interface{}
	// oldProvenance lets us re-assert command-line flag precedence after the
	// file/env layers below have been reloaded (see the restore step)
	var oldProvenance map[string]Source
	var oldTrail map[string][]Source
	var oldChanged bool
	var oldChangeVersion uint64

	// Get a snapshot of the current configuration
	c.configMutex.Lock()
	oldChanged = c.changed
	oldChangeVersion = c.changeVersion
	oldConfig = make(map[string]interface{})
	for k, v := range c.configData {
		oldConfig[k] = v
	}
	oldProvenance = make(map[string]Source, len(c.provenance))
	for k, v := range c.provenance {
		oldProvenance[k] = v
	}
	oldTrail = make(map[string][]Source, len(c.provenanceTrail))
	for k, v := range c.provenanceTrail {
		cp := make([]Source, len(v))
		copy(cp, v)
		oldTrail[k] = cp
	}
	c.resetToDefaultsLocked()
	// Reset the changed flag
	c.changed = false
	c.configMutex.Unlock()

	var err error

	// Attempt to reload configuration from file (loadConfig handles its own locking)
	if c.configHandler != nil {
		if err = c.loadConfig(false); err != nil {
			c.log().Error("cfggo: failed to reload configuration source", "err", err)

			// Rollback to old configuration on error. Restore provenance too so
			// it stays consistent with the values after a failed reload
			c.configMutex.Lock()
			c.configData = oldConfig
			c.provenance = oldProvenance
			c.provenanceTrail = oldTrail
			c.changed = oldChanged
			c.changeVersion = oldChangeVersion
			c.configMutex.Unlock()
			return err
		}
	}

	// Reload from environment variables
	if err = c.loadFromEnv(); err != nil {
		c.log().Error("cfggo: failed to reload environment variables", "err", err)
		c.configMutex.Lock()
		c.configData = oldConfig
		c.provenance = oldProvenance
		c.provenanceTrail = oldTrail
		c.changed = oldChanged
		c.changeVersion = oldChangeVersion
		c.configMutex.Unlock()
		return err
	}

	// Check if flags have been parsed before calling parseFlags
	var flagsParsed bool
	c.configMutex.RLock()
	flagsParsed = c.FlagSet != nil && c.FlagSet.Parsed()
	c.configMutex.RUnlock()

	// Reload from flags if they've been parsed
	if flagsParsed {
		if err := c.parseFlags(); err != nil {
			c.log().Warn("cfggo: could not re-parse command-line flags during reload", "err", err)
		}
	}

	// Re-assert runtime override precedence. The standard flag package will not
	// re-run an already-parsed FlagSet (parseFlags above early-returns), so the
	// file and environment layers reloaded above can otherwise clobber values
	// supplied on the command line. Programmatic Set values are also runtime
	// overrides and must survive reloads until the caller changes them again
	for key, src := range oldProvenance {
		if src != SourceFlag && src != SourceSet {
			continue
		}
		if v, ok := oldConfig[key]; ok {
			if err := c.applyLoaded(key, v, src); err != nil {
				c.log().Warn("cfggo: could not restore runtime override during reload", "key", key, "source", src, "err", err)
			}
		}
	}

	// The accessor closures installed during Init read c.configData live on
	// every call, so the reloaded values are already visible without
	// reinstalling them. Re-running replaceConfigFuncs here would rewrite the
	// struct func fields, racing with any goroutine currently calling an
	// accessor (and with a concurrent reload)

	// Validate configuration after reloading. A failed reload must leave the
	// last known-good values live; otherwise validators only report that the
	// service has already been poisoned by bad config.
	if err = c.Validate(); err != nil {
		c.log().Warn("cfggo: configuration validation failed after reload; keeping previous values", "err", err)
		c.configMutex.Lock()
		c.configData = oldConfig
		c.provenance = oldProvenance
		c.provenanceTrail = oldTrail
		c.changed = oldChanged
		c.changeVersion = oldChangeVersion
		c.configMutex.Unlock()
		return err
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

func (c *Structure) resetToDefaultsLocked() {
	if c.defaultData == nil {
		return
	}
	c.configData = make(map[string]interface{}, len(c.defaultData))
	c.provenance = make(map[string]Source, len(c.defaultData))
	c.provenanceTrail = nil
	for k, v := range c.defaultData {
		c.configData[k] = v
		c.provenance[k] = SourceDefault
	}
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
	for k, oldVal := range old {
		if _, ok := c.configData[k]; !ok {
			changes = append(changes, Change{
				Key:    k,
				Old:    oldVal,
				New:    nil,
				Source: SourceUnknown,
			})
		}
	}
	return changes
}

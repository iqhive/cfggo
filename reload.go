package cfggo

// Reload reloads the configuration from all sources
func (c *Structure) Reload() error {
	// First, make a copy of the current configuration for potential rollback
	var oldConfig map[string]interface{}

	// Get a snapshot of the current configuration
	configMutex.Lock()
	oldConfig = make(map[string]interface{})
	for k, v := range c.configData {
		oldConfig[k] = v
	}
	// Reset the changed flag
	c.changed = false
	configMutex.Unlock()

	var err error

	// Attempt to reload configuration from file (loadConfig handles its own locking)
	if c.configHandler != nil {
		if err = c.loadConfig(false); err != nil {
			Logger.Errorf("Failed to reload configuration from file: %v", err)

			// Rollback to old configuration on error
			configMutex.Lock()
			c.configData = oldConfig
			configMutex.Unlock()
			return err
		}
	}

	// Reload from environment variables
	c.loadFromEnv()

	// Check if flags have been parsed before calling parseFlags
	var flagsParsed bool
	configMutex.RLock()
	flagsParsed = c.FlagSet != nil && c.FlagSet.Parsed()
	configMutex.RUnlock()

	// Reload from flags if they've been parsed
	if flagsParsed {
		c.parseFlags()
	}

	// Update the config functions
	c.replaceConfigFuncs()

	// Validate configuration after reloading
	if err = c.Validate(); err != nil {
		Logger.Warnf("Configuration validation failed after reload: %v", err)
		// We don't return this error as it's just a warning
	}

	return nil
}

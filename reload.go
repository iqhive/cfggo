package cfggo

// Reload reloads the configuration from all sources
func (c *Structure) Reload() error {
    // Save the current configuration for rollback in case of error
    configMutex.RLock()
    oldConfig := make(map[string]interface{})
    for k, v := range c.configData {
        oldConfig[k] = v
    }
    configMutex.RUnlock()
    
    // Reset the changed flag
    configMutex.Lock()
    c.changed = false
    configMutex.Unlock()
    
    // Attempt to reload configuration from file
    if c.configHandler != nil {
        if err := c.loadConfig(); err != nil {
            // Rollback to old configuration on error
            configMutex.Lock()
            c.configData = oldConfig
            configMutex.Unlock()
            return err
        }
    }
    
    // Reload from environment variables
    c.loadFromEnv()
    
    // Reload from flags if they've been parsed
    if c.FlagSet.Parsed() {
        c.parseFlags()
    }
    
    // Update the config functions
    c.replaceConfigFuncs()
    
    return nil
}

package cfggo

// createFlags registers a flag for every key in the config map and, when any
// key is secret, wraps the flag set's output so the flag package's own error
// text cannot echo a secret value.
func (c *Structure) createFlags() {
	c.registerConfigPathFlag()
	for key, value := range c.configData {
		configDescription := c.helpTag(key)
		c.newFlag(key, value, configDescription)
	}
	c.installRedactingOutput()
}

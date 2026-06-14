package cfggo

// createFlags registers a flag for every key in the config map.
func (c *Structure) createFlags() {
	for key, value := range c.configData {
		configDescription := c.helpTag(key)
		c.newFlag(key, value, configDescription)
	}
}

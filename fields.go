package cfggo

// createFlags registers a flag for every key in the config map.
func (c *Structure) createFlags() {
	for key, value := range c.configData {
		configDescription := c.GetHelpTag(key)
		c.NewFlag(key, value, configDescription)
	}
}

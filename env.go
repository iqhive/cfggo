package cfggo

import (
	"os"
	"reflect"
	"strings"
)

func (c *Structure) loadFromEnv() {
	if c.skipEnv {
		Logger.Debug("loadFromEnv: skipping environment variables")
		return
	}

	// Use a recursive function to handle nested structures
	var loadEnvForKey func(string)
	loadEnvForKey = func(prefix string) {
		// First, gather all the keys and their types without holding locks
		type keyInfo struct {
			key       string
			valueType reflect.Type
		}

		var keysToProcess []keyInfo

		configMutex.RLock()
		// Create a copy of the keys to avoid holding the lock during the entire operation
		for key, value := range c.configData {
			if strings.HasPrefix(key, prefix) {
				// Only process keys at the current nesting level
				if prefix == "" || (strings.Count(key[len(prefix)+1:], ".") == 0) {
					keysToProcess = append(keysToProcess, keyInfo{
						key:       key,
						valueType: reflect.TypeOf(value),
					})
				}
			}
		}
		configMutex.RUnlock()

		// Now process each key without holding any locks
		for _, info := range keysToProcess {
			envVar := strings.ToUpper(strings.ReplaceAll(info.key, ".", "_"))
			if value, exists := os.LookupEnv(envVar); exists {
				// Create the dynamicVar with the saved type information
				dv := &dynamicVar{config: c, name: info.key, want: info.valueType}

				// Set the value - this will acquire its own locks
				if err := dv.Set(value); err != nil {
					Logger.Infof("Error setting config from environment variable %s=(%v): %v", envVar, value, err)
				} else {
					Logger.Debugf("Set config from environment variable %s=(%v)", envVar, value)

					// Mark that configuration has changed
					configMutex.Lock()
					c.changed = true
					configMutex.Unlock()
				}
			}
		}
	}

	// Start with top-level keys
	loadEnvForKey("")

	// Process each level of nesting
	// First, gather all the nested prefixes without holding locks
	var nestedPrefixes []string

	configMutex.RLock()
	prefixMap := make(map[string]bool)
	for key := range c.configData {
		if strings.Contains(key, ".") {
			prefix := key[:strings.LastIndex(key, ".")]
			prefixMap[prefix] = true
		}
	}
	configMutex.RUnlock()

	// Convert to slice
	for prefix := range prefixMap {
		nestedPrefixes = append(nestedPrefixes, prefix)
	}

	// Now process each nested prefix
	for _, prefix := range nestedPrefixes {
		loadEnvForKey(prefix + ".")
	}
}

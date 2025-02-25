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
		for key := range c.configData {
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			
			// Only process keys at the current nesting level
			if prefix != "" && strings.Count(key[len(prefix)+1:], ".") > 0 {
				continue
			}
			
			envVar := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
			if value, exists := os.LookupEnv(envVar); exists {
				dv := &dynamicVar{config: c, name: key, want: reflect.TypeOf(c.configData[key])}
				if err := dv.Set(value); err != nil {
					Logger.Info("Error setting config from environment variable %s=(%v): %v", envVar, value, err)
				}
			}
		}
	}
	
	// Start with top-level keys
	loadEnvForKey("")
	
	// Process each level of nesting
	for key := range c.configData {
		if strings.Contains(key, ".") {
			prefix := key[:strings.LastIndex(key, ".")]
			loadEnvForKey(prefix + ".")
		}
	}
}

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
	for key := range c.configData {
		envVar := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		if value, exists := os.LookupEnv(envVar); exists {
			// Logger.Debug("found environment variable %s with value %s", envVar, value)
			dv := &dynamicVar{config: c, name: key, want: reflect.TypeOf(c.configData[key])}
			if err := dv.Set(value); err != nil {
				Logger.Info("Error setting config from environment variable %s=(%v): %v", envVar, value, err)
			}
		}
	}
}

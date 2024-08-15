package cfggo

import (
	"flag"
	"reflect"
)

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	//c.configData[configVarName] = defaultValue
	if flag.Lookup(configVarName) != nil {
		Logger.Error("Flag %s is already set, skipping...\n", configVarName)
		return
	}
	flag.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(c.configData[configVarName])}, configVarName, configDescription)
}

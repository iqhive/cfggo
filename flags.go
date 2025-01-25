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
	// Logger.Info("NewFlag " + configVarName + " " + fmt.Sprintf("%v", defaultValue))

	if flag.Lookup(configVarName) != nil {
		Logger.Error("Flag (" + configVarName + ") is already set, skipping...\n")
		return
	}
	if c.configData[configVarName] == nil {
		Logger.Warn("configData[" + configVarName + "] is not set, using default value for type")
		flag.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(defaultValue)}, configVarName, configDescription)
	} else {
		flag.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(c.configData[configVarName])}, configVarName, configDescription)
	}
}

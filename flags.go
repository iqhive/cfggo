package cfggo

import (
	"fmt"
	"os"
	"reflect"
	"sync"
)

var autoParseOnce sync.Once

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	//c.configData[configVarName] = defaultValue
	// Logger.Info("NewFlag " + configVarName + " " + fmt.Sprintf("%v", defaultValue))

	if c.FlagSet.Lookup(configVarName) != nil {
		Logger.Error("Flag (" + configVarName + ") is already set, skipping...\n")
		return
	}
	if c.configData[configVarName] == nil {
		Logger.Warn("configData[" + configVarName + "] is not set, using default value for type")
		c.FlagSet.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(defaultValue)}, configVarName, configDescription)
	} else {
		c.FlagSet.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(c.configData[configVarName])}, configVarName, configDescription)
	}
}

func (c *Structure) parseFlags() {
	fmt.Println("parseFlags")
	fmt.Println(os.Args[1:])
	// Auto-parse flags once if not already parsed
	autoParseOnce.Do(func() {
		if !c.FlagSet.Parsed() {
			if err := c.FlagSet.Parse(os.Args[1:]); err != nil {
				Logger.Error("error parsing flags: %v", err)
			}
		}
	})
}

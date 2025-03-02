package cfggo

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"sync"
)

var autoParseOnce sync.Once

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	if c.parent == nil {
		c.InitSelf()
	}

	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	if c.FlagSet.Lookup(configVarName) != nil {
		Logger.Error("Flag (%s) is already set, skipping...\n", configVarName)
		return
	}

	// Only register with the dedicated FlagSet, not the global one
	if c.configData[configVarName] == nil {
		Logger.Warn("configData[%s] is not set, using default value for type", configVarName)
		c.FlagSet.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(defaultValue)}, configVarName, configDescription)
	} else {
		c.FlagSet.Var(&dynamicVar{config: c, name: configVarName, want: reflect.TypeOf(c.configData[configVarName])}, configVarName, configDescription)
	}
}

func (c *Structure) parseFlags() {
	if !c.FlagSet.Parsed() {
		// Filter out Go test flags
		args := filterTestFlags(os.Args[1:])
		if err := c.FlagSet.Parse(args); err != nil {
			Logger.Error("error parsing flags: %v", err)
		}
	}
}

// GetFlagSet returns the FlagSet used by this configuration
// This allows applications to register the FlagSet with their own flag parsing system
func (c *Structure) GetFlagSet() *flag.FlagSet {
	if c.parent == nil {
		c.InitSelf()
	}

	return c.FlagSet
}

// filterTestFlags removes Go test flags (starting with -test.) from arguments
func filterTestFlags(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-test.") {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

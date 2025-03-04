package cfggo

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
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

	// Special handling for boolean flags
	if boolVal, isBool := defaultValue.(bool); isBool {
		// Use BoolVar for boolean values, allowing standalone flags like --boolfield
		boolPtr := new(bool)
		*boolPtr = boolVal
		c.FlagSet.BoolVar(boolPtr, configVarName, boolVal, configDescription)

		// Track the original boolean flag value so we can detect changes
		c.configData[configVarName] = boolVal

		// Register a callback for when flags are parsed
		go func(name string, ptr *bool) {
			// We need to wait for flags to be parsed before updating the config
			c.waitForFlagParsed()
			if *ptr != boolVal {
				// The flag value changed, update the config
				c.configData[name] = *ptr
			}
		}(configVarName, boolPtr)
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

// Helper method to wait for flags to be parsed
func (c *Structure) waitForFlagParsed() {
	// Create a ticker to periodically check if flags have been parsed
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Watch for up to 1 second to avoid infinite wait
	timeout := time.After(1 * time.Second)

	for {
		select {
		case <-ticker.C:
			if c.FlagSet != nil && c.FlagSet.Parsed() {
				return
			}
		case <-timeout:
			// Don't wait forever, just return
			return
		}
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

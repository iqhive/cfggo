package cfggo

import (
	"flag"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/iqhive/cfggo/internal/flags"
)

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.Lock()
	defer configMutex.Unlock()

	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	if c.FlagSet.Lookup(configVarName) != nil {
		Logger.Errorf("Flag (%s) is already set, skipping...\n", configVarName)
		return
	}

	// Special handling for boolean flags
	if boolVal, isBool := defaultValue.(bool); isBool {
		c.configData[configVarName] = boolVal
		boolPtr := &boolVal

		boolCallback := func(parsedValue bool) {
			// parseFlags already holds the lock when invoking callbacks
			c.configData[configVarName] = parsedValue
			c.changed = true
		}

		if c.boolCallbacks == nil {
			c.boolCallbacks = make(map[string]func(bool))
		}
		c.boolCallbacks[configVarName] = boolCallback
		c.FlagSet.BoolVar(boolPtr, configVarName, boolVal, configDescription)
		return
	}

	if c.configData[configVarName] == nil {
		Logger.Warnf("configData[%s] is not set, using default value for type", configVarName)
		c.configData[configVarName] = defaultValue
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(defaultValue),
			Setter: c.createSetter(configVarName),
		}
		c.FlagSet.Var(dvar, configVarName, configDescription)
	} else {
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(c.configData[configVarName]),
			Setter: c.createSetter(configVarName),
		}
		c.FlagSet.Var(dvar, configVarName, configDescription)
	}
}

// createSetter returns a closure that acquires configMutex and stores the value.
func (c *Structure) createSetter(key string) func(interface{}) error {
	return func(value interface{}) error {
		configMutex.Lock()
		defer configMutex.Unlock()
		c.changed = true
		return c.set(key, value)
	}
}

// waitForFlagParsed blocks until the FlagSet has been parsed or a 2-second
// timeout is reached.
func (c *Structure) waitForFlagParsed() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			configMutex.RLock()
			parsed := c.FlagSet != nil && c.FlagSet.Parsed()
			configMutex.RUnlock()
			if parsed {
				return
			}
		case <-timeout:
			Logger.Debug("Timeout waiting for flags to be parsed")
			return
		}
	}
}

func (c *Structure) parseFlags() {
	configMutex.Lock()
	defer configMutex.Unlock()

	if c.FlagSet.Parsed() {
		Logger.Infof("parseFlags: flags already parsed %s", c.name)
		return
	}

	args := flags.FilterTestFlags(os.Args[1:])

	// Temporarily release the lock during parsing to avoid deadlocks with Set().
	configMutex.Unlock()
	var parseErr error
	if parseErr = c.FlagSet.Parse(args); parseErr != nil {
		Logger.Errorf("error parsing flags: %v", parseErr)
	}
	configMutex.Lock()

	if c.boolCallbacks != nil && parseErr == nil {
		c.FlagSet.Visit(func(f *flag.Flag) {
			if callback, exists := c.boolCallbacks[f.Name]; exists {
				value, err := strconv.ParseBool(f.Value.String())
				if err == nil {
					callback(value)
				} else {
					Logger.Errorf("Failed to parse bool flag %s: %v", f.Name, err)
				}
			}
		})
	}

	if parseErr == nil {
		c.changed = true
	}
}

// GetFlagSet returns the FlagSet used by this configuration, initialising it
// via InitSelf if necessary.
func (c *Structure) GetFlagSet() *flag.FlagSet {
	if c.parent == nil {
		c.InitSelf()
	}
	return c.FlagSet
}

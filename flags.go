package cfggo

import (
	"flag"
	"os"
	"reflect"
	"strings"
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

	// Special handling for boolean flags: register a bool-aware ConfigVar so the
	// value propagates to the config map during Parse (whether cfggo parses its
	// own private FlagSet or the host parses flag.CommandLine), and so the
	// standard flag package allows the "--flag" / "--flag=true" forms.
	if boolVal, isBool := defaultValue.(bool); isBool {
		c.configData[configVarName] = boolVal
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(boolVal),
			Setter: c.createSetter(configVarName),
			IsBool: true,
		}
		c.FlagSet.Var(dvar, configVarName, configDescription)
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

	// When WithIgnoreUnknownVars is enabled, only parse the flags cfggo knows
	// about. Flags owned by other libraries or simple typos are filtered out so
	// they never cause a parse failure (or a hard process exit). Otherwise the
	// default flag.ExitOnError behaviour applies and an unknown flag aborts
	if c.ignoreUnknownVars {
		if !c.externalFlagSet {
			c.FlagSet.Init(c.FlagSet.Name(), flag.ContinueOnError)
		}
		args = c.filterKnownFlags(args)
	}

	// Temporarily release the lock during parsing to avoid deadlocks with Set().
	configMutex.Unlock()
	var parseErr error
	if parseErr = c.FlagSet.Parse(args); parseErr != nil {
		Logger.Errorf("error parsing flags: %v", parseErr)
	}
	configMutex.Lock()

	// Leftover positional arguments usually indicate a "--bool value" mistake
	// (boolean flags require the "--bool=value" form) or a stray argument.
	if parseErr == nil {
		if rest := c.FlagSet.Args(); len(rest) > 0 {
			Logger.Warnf("cfggo: ignoring unexpected positional arguments after flag parsing: %v "+
				"(note: boolean flags must use the --flag=value form to set an explicit value)", rest)
		}
	}

	if parseErr == nil {
		c.changed = true
	}
}

// filterKnownFlags returns only the argument tokens that correspond to flags
// registered on c.FlagSet. Unknown flags (and their separate values) are
// dropped so they neither abort parsing nor terminate the process. This lets
// cfggo coexist with libraries that register flags elsewhere (e.g. on
// flag.CommandLine) when cfggo is using its own private flag set
func (c *Structure) filterKnownFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// "--" terminates flag parsing; pass it and everything after through.
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}

		// Non-flag (positional) argument, or a bare "-".
		if len(arg) < 2 || arg[0] != '-' {
			out = append(out, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		hasInlineValue := false
		if idx := strings.IndexByte(name, '='); idx != -1 {
			name = name[:idx]
			hasInlineValue = true
		}

		f := c.FlagSet.Lookup(name)
		if f == nil {
			Logger.Debugf("cfggo: ignoring unrecognized flag %q (not defined on this config)", arg)
			// For "--unknown value", also drop the following value token so it is
			// not misread as a positional argument (which would stop parsing)
			if !hasInlineValue && i+1 < len(args) {
				if next := args[i+1]; len(next) == 0 || next[0] != '-' {
					i++
				}
			}
			continue
		}

		out = append(out, arg)

		// A known non-bool flag with no inline value consumes the next token
		if !hasInlineValue && i+1 < len(args) && !isBoolFlag(f) {
			out = append(out, args[i+1])
			i++
		}
	}
	return out
}

// isBoolFlag reports whether the given flag behaves like a boolean flag
func isBoolFlag(f *flag.Flag) bool {
	if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
		return bf.IsBoolFlag()
	}
	return false
}

// GetFlagSet returns the FlagSet used by this configuration, initialising it
// via InitSelf if necessary.
func (c *Structure) GetFlagSet() *flag.FlagSet {
	if c.parent == nil {
		c.InitSelf()
	}
	return c.FlagSet
}

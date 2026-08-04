package cfggo

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/iqhive/cfggo/internal/flags"
)

// NewFlag creates a new configuration item, using the type of the defaultValue
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	c.ensureInit()
	c.newFlag(configVarName, defaultValue, configDescription)
}

func (c *Structure) newFlag(configVarName string, defaultValue interface{}, configDescription string) {
	c.ensureFlagSet()

	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	flagName := c.flagNamePrefix + configVarName
	if c.flagSet.Lookup(flagName) != nil {
		c.log().Error("cfggo: flag already registered, skipping", "flag", flagName)
		return
	}

	// Special handling for boolean flags: register a bool-aware ConfigVar so the
	// value propagates to the config map during Parse (whether cfggo parses its
	// own private flag set or the host parses flag.CommandLine), and so the
	// standard flag package allows the "--flag" / "--flag=true" forms.
	if boolVal, isBool := defaultValue.(bool); isBool {
		// Only seed the default when the key has no value yet, mirroring the
		// non-bool path below: a value already loaded from a file, the
		// environment, or a Set call must not be silently overwritten
		if _, exists := c.configData[configVarName]; !exists {
			c.configData[configVarName] = boolVal
		}
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(boolVal),
			Setter: c.createSetter(configVarName),
			IsBool: true,
		}
		c.flagSet.Var(dvar, flagName, configDescription)
		return
	}

	if c.configData[configVarName] == nil {
		c.log().Warn("cfggo: config key not set, using default value for type", "key", configVarName)
		c.configData[configVarName] = defaultValue
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(defaultValue),
			Setter: c.createSetter(configVarName),
		}
		c.flagSet.Var(dvar, flagName, configDescription)
	} else {
		dvar := &flags.ConfigVar{
			Name:   configVarName,
			Want:   reflect.TypeOf(c.configData[configVarName]),
			Setter: c.createSetter(configVarName),
		}
		c.flagSet.Var(dvar, flagName, configDescription)
	}
}

// createSetter returns a closure that acquires the instance's configMutex and
// stores the value.
func (c *Structure) createSetter(key string) func(interface{}) error {
	return func(value interface{}) error {
		if err := c.validateValueForKey(key, value, SourceFlag); err != nil {
			return c.WrapError(err, ErrCodeInvalidArgument, "key %q from %s failed validation", key, SourceFlag)
		}

		c.configMutex.Lock()
		defer c.configMutex.Unlock()
		if err := c.set(key, value); err != nil {
			return c.WrapError(err, ErrCodeInvalidArgument, "key %q from %s", key, SourceFlag)
		}
		c.markChangedLocked()
		c.recordSourceLocked(key, SourceFlag)
		return nil
	}
}

func (c *Structure) parseFlags() error {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	if c.flagSet.Parsed() {
		c.log().Info("cfggo: flags already parsed")
		return nil
	}

	args := flags.FilterTestFlags(os.Args[1:])

	// When WithIgnoreUnknownVars is enabled, only parse the flags cfggo knows
	// about. Flags owned by other libraries or simple typos are filtered out so
	// they never cause a parse failure (or a hard process exit). Otherwise the
	// default flag.ExitOnError behaviour applies and an unknown flag aborts
	if c.ignoreUnknownVars {
		if !c.externalFlagSet {
			c.flagSet.Init(c.flagSet.Name(), flag.ContinueOnError)
		}
		args = filterKnownFlags(c.flagSet, args, func(arg string) {
			c.log().Debug("cfggo: ignoring unrecognized flag (not defined on this config)", "flag", arg)
		})
	} else if name, ok := firstUnknownFlag(c.flagSet, args); ok {
		if suggestion := c.suggestKey(name); suggestion != "" {
			return c.WrapError(
				wrapKind(ErrUnknownKey, fmt.Errorf("flag provided but not defined: -%s (did you mean -%s?)", name, suggestion)),
				ErrCodeNotFound,
				"",
			)
		}
	}

	// Collapse "--bool value" into "--bool=value" for known boolean flags. The
	// standard flag package stops parsing at the first non-flag token, so an
	// uncollapsed boolean value (e.g. "--boolval true") would otherwise be read
	// as a positional argument and silently drop every flag that follows it.
	args = normalizeBoolFlagArgs(c.flagSet, args)

	// Temporarily release the lock during parsing to avoid deadlocks with Set().
	c.configMutex.Unlock()
	var parseErr error
	if parseErr = c.flagSet.Parse(args); parseErr != nil {
		c.log().Error("cfggo: error parsing flags", "err", parseErr)
	}
	c.configMutex.Lock()

	// Leftover positional arguments usually indicate a "--bool value" mistake
	// (boolean flags require the "--bool=value" form) or a stray argument.
	if parseErr == nil {
		if rest := c.flagSet.Args(); len(rest) > 0 {
			c.log().Warn("cfggo: ignoring unexpected positional arguments after flag parsing "+
				"(boolean flags must use the --flag=value form to set an explicit value)", "args", rest)
		}
	}

	if parseErr == nil {
		c.markChangedLocked()
	}
	if parseErr != nil {
		return c.WrapError(parseErr, ErrCodeInvalidArgument, "parse command-line flags")
	}
	return nil
}

func (c *Structure) firstUnknownFlag(args []string) (string, bool) {
	return firstUnknownFlag(c.flagSet, args)
}

func firstUnknownFlag(fs *flag.FlagSet, args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return "", false
		}
		if len(arg) < 2 || arg[0] != '-' {
			return "", false
		}
		// A negative number (eg "-1" or "-2.5") is a value or positional
		// argument, not a flag; the standard flag package would stop here too
		if isNegativeNumber(arg) {
			return "", false
		}

		name := trimFlagDashes(arg)
		if name == "" {
			return "", false
		}
		hasInlineValue := false
		if idx := strings.IndexByte(name, '='); idx != -1 {
			name = name[:idx]
			hasInlineValue = true
		}

		f := fs.Lookup(name)
		if f == nil {
			return name, true
		}
		if !hasInlineValue && i+1 < len(args) && !isBoolFlag(f) {
			i++
		}
	}
	return "", false
}

// filterKnownFlags returns only the argument tokens that correspond to flags
// registered on c.flagSet. Unknown flags (and their separate values) are
// dropped so they neither abort parsing nor terminate the process. This lets
// cfggo coexist with libraries that register flags elsewhere (e.g. on
// flag.CommandLine) when cfggo is using its own private flag set
func (c *Structure) filterKnownFlags(args []string) []string {
	return filterKnownFlags(c.flagSet, args, func(arg string) {
		c.log().Debug("cfggo: ignoring unrecognized flag (not defined on this config)", "flag", arg)
	})
}

func filterKnownFlags(fs *flag.FlagSet, args []string, debug ...func(string)) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// "--" terminates flag parsing; pass it and everything after through.
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}

		// Non-flag (positional) argument, a bare "-", or a negative number.
		if len(arg) < 2 || arg[0] != '-' || isNegativeNumber(arg) {
			out = append(out, arg)
			continue
		}

		name := trimFlagDashes(arg)
		hasInlineValue := false
		if idx := strings.IndexByte(name, '='); idx != -1 {
			name = name[:idx]
			hasInlineValue = true
		}

		f := fs.Lookup(name)
		if f == nil {
			if len(debug) > 0 && debug[0] != nil {
				debug[0](arg)
			}
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

// normalizeBoolFlagArgs rewrites "--bool value" into "--bool=value" for any
// known boolean flag whose value is supplied as a separate boolean-literal
// token. This is required because Go's flag package treats a boolean flag's
// following token as a positional argument and stops parsing at it, which would
// otherwise cause every subsequent flag to be ignored (e.g. the trailing
// "--stringval x" in "--boolval true --stringval x"). Tokens that are not
// boolean literals are left untouched so genuinely stray arguments still
// surface via the existing positional-argument warning.
func (c *Structure) normalizeBoolFlagArgs(args []string) []string {
	return normalizeBoolFlagArgs(c.flagSet, args)
}

func normalizeBoolFlagArgs(fs *flag.FlagSet, args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// "--" terminates flag parsing; pass it and everything after through.
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}

		// Non-flag (positional) argument, a bare "-", or a negative number.
		if len(arg) < 2 || arg[0] != '-' || isNegativeNumber(arg) {
			out = append(out, arg)
			continue
		}

		name := trimFlagDashes(arg)
		// Already in "--flag=value" form; nothing to collapse.
		if strings.IndexByte(name, '=') != -1 {
			out = append(out, arg)
			continue
		}

		f := fs.Lookup(name)
		if f != nil && isBoolFlag(f) && i+1 < len(args) {
			// be flexible with the values we support for bool flags, because
			// some config var names may cause end-users to supply "yes/no/y/n"
			// instead of "true/false/t/f"
			if boolval, boolErr := customParseBool(args[i+1]); boolErr == nil {
				if boolval {
					out = append(out, arg+"=true")
				} else {
					out = append(out, arg+"=false")
				}
				i++
				continue
			}
		}

		out = append(out, arg)
	}
	return out
}

// trimFlagDashes strips the leading "-" or "--" from a flag token, matching
// the standard flag package (which accepts at most two dashes) instead of
// stripping every leading dash
func trimFlagDashes(arg string) string {
	name := strings.TrimPrefix(arg, "-")
	name = strings.TrimPrefix(name, "-")
	return name
}

// isNegativeNumber reports whether arg is a negative numeric literal such as
// "-1" or "-2.5", which is an argument value rather than a flag
func isNegativeNumber(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	_, err := strconv.ParseFloat(arg[1:], 64)
	return err == nil
}

// replacement for strconv.ParseBool that also supports yes/y/no/n
func customParseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "t", "yes", "y", "1":
		return true, nil
	case "false", "f", "no", "n", "0":
		return false, nil
	}
	return false, fmt.Errorf("cannot parse bool %q", s)
}

// isBoolFlag reports whether the given flag behaves like a boolean flag
func isBoolFlag(f *flag.Flag) bool {
	if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
		return bf.IsBoolFlag()
	}
	return false
}

// GetFlagSet returns the FlagSet used by this configuration, initialising it
// via InitSelf if necessary. When the configuration was created WithoutFlags,
// the private flag set is created lazily on first access so the returned value
// is never nil.
func (c *Structure) GetFlagSet() *flag.FlagSet {
	c.ensureInit()
	c.ensureFlagSet()
	return c.flagSet
}

// ensureFlagSet lazily creates the private flag set. It is a no-op once a set
// exists (including a caller-supplied external one). This backs the WithoutFlags
// path, where Init deliberately skips creating the set, while keeping
// GetFlagSet/NewFlag usable if a caller still wants flags afterwards.
func (c *Structure) ensureFlagSet() {
	if c.flagSet != nil {
		return
	}
	c.flagSet = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	c.flagSet.Usage = func() {
		fmt.Fprintf(c.flagSet.Output(), "Usage of %s:\n", os.Args[0])
		c.flagSet.PrintDefaults()
	}
}

package cfggo

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/iqhive/cfggo/internal/flags"
)

// NewFlag registers an additional configuration key that is not backed by a
// struct field, with a default value (which fixes the key's type; nil means
// "infer from the text") and help text. Read it with Get, Value or MustValue.
//
// Call it before Init: the key then takes part in every layer exactly like a
// struct-backed key (file, environment, command-line flag, Set, Save,
// Reload) and is not reported as unrecognized. After Init the key is added
// with its default, its environment variable is read once (unless the
// configuration was initialised WithoutEnv), a flag is registered (on the
// flag set in use, or a private one created on demand as GetFlagSet does),
// and Set can change it; a private flag set that Init has already parsed is
// not parsed again
func (c *Structure) NewFlag(configVarName string, defaultValue interface{}, configDescription string) {
	c.configMutex.Lock()
	if c.extraKeys == nil {
		c.extraKeys = make(map[string]string)
	}
	c.extraKeys[configVarName] = configDescription
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	if existing, exists := c.configData[configVarName]; !exists || (existing == nil && defaultValue != nil) {
		c.configData[configVarName] = cloneMutableInterface(defaultValue)
		c.recordSourceLocked(configVarName, SourceDefault)
		if c.defaultData != nil {
			c.defaultData[configVarName] = cloneMutableInterface(defaultValue)
		}
	}
	c.configMutex.Unlock()

	if !c.initialized.Load() {
		// Init registers the flag on the final flag set together with every
		// other key and applies the file, environment and flag layers
		return
	}
	// Register the flag now (creating the private flag set lazily, as
	// GetFlagSet does) so a host that has not parsed yet still picks it up
	c.newFlag(configVarName, defaultValue, configDescription)
	if !c.skipEnv {
		if err := c.applyEnvForKey(configVarName); err != nil {
			c.log().Warn("cfggo: NewFlag: environment value not applied", "key", configVarName, "err", err)
		}
	}
}

func (c *Structure) newFlag(configVarName string, defaultValue interface{}, configDescription string) {
	c.ensureFlagSet()

	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	// The flag name may carry a prefix (set only by Group) so several
	// configurations can share one flag set; the configuration key itself is
	// never prefixed, so plans, accessors, and file/env keys are unaffected.
	flagName := c.flagNamePrefix + configVarName

	if existing := c.flagSet.Lookup(flagName); existing != nil {
		if cv, ok := existing.Value.(*flags.ConfigVar); ok && cv.Name == configVarName {
			// Already registered for this key (eg a retried Init on a host
			// flag set): nothing to do
			return
		}
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

	current, exists := c.configData[configVarName]
	if !exists || (current == nil && defaultValue != nil) {
		c.configData[configVarName] = defaultValue
		current = defaultValue
	}
	want := reflect.TypeOf(current)
	if want == nil {
		// An untyped nil (eg a func() interface{} field with no default) has
		// no runtime type; use the type the struct declares so the flag still
		// accepts a value instead of failing with a nil target type. The
		// config lock is held here, so use the locked variant
		want = c.declaredTypeLocked(configVarName)
	}
	if want == nil {
		c.log().Debug("cfggo: flag not registered: key has neither a value nor a declared type", "key", configVarName)
		return
	}
	dvar := &flags.ConfigVar{
		Name:     configVarName,
		Want:     want,
		Setter:   c.createSetter(configVarName),
		IsSecret: c.isSecretKey(configVarName),
	}
	c.flagSet.Var(dvar, flagName, configDescription)
}

// registerConfigPathFlag registers the bootstrap config-path flag supplied to
// WithFileConfigParamName so it is not reported as unknown during parsing. It
// runs when the other flags are registered, on the final flag set, so a
// WithFlagSet applied after WithFileConfigParamName cannot discard it.
// Registering a flag name twice on one set panics in the standard flag
// package, so an already-registered name (eg the host's own flag on a shared
// set, or a retried Init) is left as-is
func (c *Structure) registerConfigPathFlag() {
	if c.configPathFlag == "" || c.flagSet == nil {
		return
	}
	if c.flagSet.Lookup(c.configPathFlag) == nil {
		c.flagSet.String(c.configPathFlag, "", "path to the configuration file")
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
		c.recordSourceLocked(key, SourceFlag)
		return nil
	}
}

func (c *Structure) parseFlags() error {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	if c.flagSet.Parsed() {
		c.log().Debug("cfggo: flags already parsed")
		return nil
	}

	args := flags.FilterTestFlags(os.Args[1:])

	// When WithIgnoreUnknownVars is enabled, only parse the flags cfggo knows
	// about. Flags owned by other libraries or simple typos are filtered out so
	// they never cause a parse failure. Otherwise an unknown flag is an error,
	// with a "did you mean" hint when a close match exists. It is always
	// returned (never a process exit) so every unknown flag behaves the same
	if c.ignoreUnknownVars {
		args = c.filterKnownFlags(args)
	} else if name, ok := c.firstUnknownFlag(args); ok {
		suffix := ""
		if suggestion := c.suggestKey(name); suggestion != "" {
			suffix = fmt.Sprintf(" (did you mean -%s?)", suggestion)
		} else if c.flagSet.Usage != nil {
			// No close match: list every known flag, as the flag package
			// would, so the user can see what is accepted
			c.flagSet.Usage()
		}
		return c.WrapError(
			wrapKind(ErrUnknownKey, fmt.Errorf("flag provided but not defined: -%s%s", name, suffix)),
			ErrCodeNotFound,
			"",
		)
	}

	// Collapse "--bool value" into "--bool=value" for known boolean flags. The
	// standard flag package stops parsing at the first non-flag token, so an
	// uncollapsed boolean value (e.g. "--boolval true") would otherwise be read
	// as a positional argument and silently drop every flag that follows it.
	args = c.normalizeBoolFlagArgs(args)

	// Temporarily release the lock during parsing to avoid deadlocks with Set().
	c.configMutex.Unlock()
	var parseErr error
	if parseErr = c.flagSet.Parse(args); parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			// Usage has already been printed by the flag package. Hand the
			// sentinel back so the host can errors.Is(err, flag.ErrHelp) and
			// exit cleanly; it is not a configuration error worth logging
			c.configMutex.Lock()
			return c.WrapError(parseErr, ErrCodeInvalidArgument, "help requested")
		}
		// The flag package echoes the rejected raw value ("invalid value "x"
		// for flag -name"); strip it for secret-tagged keys before it reaches
		// a log line or the caller
		parseErr = c.redactFlagError(parseErr)
		c.log().Error("cfggo: error parsing flags", "err", parseErr)
	}
	c.configMutex.Lock()

	// Leftover positional arguments usually indicate a "--bool value" mistake
	// (boolean flags require the "--bool=value" form) or a stray argument. Only
	// their number is logged: a stray token may be a credential meant for a flag
	if parseErr == nil {
		if rest := c.flagSet.Args(); len(rest) > 0 {
			c.log().Warn("cfggo: ignoring unexpected positional arguments after flag parsing "+
				"(boolean flags must use the --flag=value form to set an explicit value)", "count", len(rest))
		}
	}

	if parseErr != nil {
		return c.WrapError(parseErr, ErrCodeInvalidArgument, "parse command-line flags")
	}
	return nil
}

func (c *Structure) firstUnknownFlag(args []string) (string, bool) {
	return firstUnknownFlagIn(c.flagSet, args)
}

// firstUnknownFlagIn returns the name of the first argument token that is not a
// flag defined on fs.
func firstUnknownFlagIn(fs *flag.FlagSet, args []string) (string, bool) {
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
			if isHelpFlag(name) {
				// Leave -h/--help to the flag package, which prints usage and
				// returns flag.ErrHelp
				return "", false
			}
			return name, true
		}
		if !hasInlineValue && i+1 < len(args) && !isBoolFlag(f) {
			i++
		}
	}
	return "", false
}

// isHelpFlag reports whether name is one of the help flags the standard flag
// package handles itself when no such flag is defined
func isHelpFlag(name string) bool {
	return name == "h" || name == "help"
}

// flagPresentInArgs reports whether a flag named name (with or without an
// inline =value) appears in args before any "--" terminator
func flagPresentInArgs(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if len(arg) < 2 || arg[0] != '-' || isNegativeNumber(arg) {
			continue
		}
		candidate := trimFlagDashes(arg)
		if idx := strings.IndexByte(candidate, '='); idx != -1 {
			candidate = candidate[:idx]
		}
		if candidate == name {
			return true
		}
	}
	return false
}

// filterKnownFlags returns only the argument tokens that correspond to flags
// registered on c.flagSet. Unknown flags (and their separate values) are
// dropped so they neither abort parsing nor terminate the process. This lets
// cfggo coexist with libraries that register flags elsewhere (e.g. on
// flag.CommandLine) when cfggo is using its own private flag set
func (c *Structure) filterKnownFlags(args []string) []string {
	return filterKnownFlagsIn(c.flagSet, args, func(name string) {
		c.log().Debug("cfggo: ignoring unrecognized flag (not defined on this config)", "flag", name)
	})
}

// filterKnownFlagsIn is filterKnownFlags against an explicit flag set, calling
// onDrop with the bare name of each flag it discards. Only the name is
// reported, never the value: a dropped flag usually belongs to another
// component and its value may be a credential.
func filterKnownFlagsIn(fs *flag.FlagSet, args []string, onDrop func(string)) []string {
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
			if isHelpFlag(name) {
				// Keep -h/--help so the flag package can still print usage
				out = append(out, arg)
				continue
			}
			if onDrop != nil {
				onDrop(name)
			}
			// For "--unknown value", also drop the following value token so it is
			// not misread as a positional argument (which would stop parsing).
			// A negative number is a value, not the next flag
			if !hasInlineValue && i+1 < len(args) {
				if next := args[i+1]; len(next) == 0 || next[0] != '-' || isNegativeNumber(next) {
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
	return normalizeBoolFlagArgsIn(c.flagSet, args)
}

// normalizeBoolFlagArgsIn is normalizeBoolFlagArgs against an explicit flag set.
func normalizeBoolFlagArgsIn(fs *flag.FlagSet, args []string) []string {
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
//
// The set uses flag.ContinueOnError: cfggo never terminates the process from
// inside Init, so unknown flags, rejected values and --help are all returned
// as errors (the last one wrapping flag.ErrHelp)
func (c *Structure) ensureFlagSet() {
	if c.flagSet != nil {
		return
	}
	c.flagSet = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	c.flagSet.Usage = func() {
		fmt.Fprintf(c.flagSet.Output(), "Usage of %s:\n", os.Args[0])
		c.flagSet.PrintDefaults()
	}
}

// --- Secret redaction for flag errors -----------------------------------------

// flagValuePattern matches the messages the standard flag package builds when a
// flag's Set rejects a value. Both carry the raw value, %q-quoted:
//
//	invalid value "RAW" for flag -name: ...
//	invalid boolean value "RAW" for -name: ...
//
// The quoted group accepts Go escape sequences so a value containing quotes
// or backslashes is still matched as a whole
var flagValuePattern = regexp.MustCompile(`invalid (?:boolean )?value ("(?:[^"\\]|\\.)*") for (?:flag )?-([^\s:=]+):`)

// redactFlagText replaces the raw value in any flag-package error text that
// refers to a secret-tagged key of this configuration with the mask used
// everywhere else. Text about other flags is left untouched
func (c *Structure) redactFlagText(s string) string {
	if c.plan == nil || !strings.Contains(s, "invalid ") {
		return s
	}
	return flagValuePattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := flagValuePattern.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		name := sub[2]
		if c.flagNamePrefix != "" {
			if !strings.HasPrefix(name, c.flagNamePrefix) {
				return m
			}
			name = strings.TrimPrefix(name, c.flagNamePrefix)
		}
		if !c.isSecretKey(name) {
			return m
		}
		return strings.Replace(m, sub[1], `"`+maskedValue+`"`, 1)
	})
}

// redactFlagError returns err with secret flag values removed from its text.
// A redacted error is rebuilt from the scrubbed message so the raw value is not
// reachable through Unwrap either; unaffected errors are returned unchanged
func (c *Structure) redactFlagError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	redacted := c.redactFlagText(msg)
	if redacted == msg {
		return err
	}
	return errors.New(redacted)
}

// RedactFlagError returns err with any secret-tagged flag value removed from
// its message.
//
// The standard flag package reports a rejected value as
// `invalid value "RAW" for flag -name: ...`, echoing the raw text. cfggo
// applies this redaction to its own parse errors and to the usage output the
// flag package prints, but a host that parses its own flag set
// (WithFlagSet / WithStandardFlags with flag.ContinueOnError) receives the
// flag package's error directly and should pass it through here before
// logging it. Errors that mention no secret flag are returned unchanged
func (c *Structure) RedactFlagError(err error) error {
	return c.redactFlagError(err)
}

// redactingWriter scrubs secret flag values from text the flag package writes
// to a flag set's output (error lines and usage)
type redactingWriter struct {
	c *Structure
	w io.Writer
}

func (r *redactingWriter) Write(p []byte) (int, error) {
	s := string(p)
	if redacted := r.c.redactFlagText(s); redacted != s {
		if _, err := io.WriteString(r.w, redacted); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return r.w.Write(p)
}

// hasSecretKeys reports whether any accessor field is tagged secret:"true"
func (c *Structure) hasSecretKeys() bool {
	if c.plan == nil {
		return false
	}
	for i := range c.plan.leaves {
		if c.plan.leaves[i].info.IsSecret {
			return true
		}
	}
	return false
}

// installRedactingOutput wraps the flag set's output so the flag package's own
// error text cannot echo a secret value. It is a no-op without secret keys and
// idempotent for this configuration; several configurations sharing one flag
// set (a Group) each add their own layer, each scrubbing the keys it owns
func (c *Structure) installRedactingOutput() {
	if c.flagSet == nil || !c.hasSecretKeys() {
		return
	}
	for w := c.flagSet.Output(); ; {
		rw, ok := w.(*redactingWriter)
		if !ok {
			break
		}
		if rw.c == c {
			return
		}
		w = rw.w
	}
	c.flagSet.SetOutput(&redactingWriter{c: c, w: c.flagSet.Output()})
}

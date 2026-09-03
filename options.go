package cfggo

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/sources"
	"github.com/iqhive/cfggo/validcfg"
)

// Option is a function that configures a Structure
type Option func(*Structure) error

// withNoop returns an Option that does nothing
func withNoop() Option {
	return func(c *Structure) error {
		return nil
	}
}

func withPrivateFlagSet(fs *flag.FlagSet) Option {
	return func(c *Structure) error {
		if fs == nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "withPrivateFlagSet: flag set must not be nil")
		}
		c.flagSet = fs
		c.externalFlagSet = false
		return nil
	}
}

// withBytesConfig makes the configuration load from an in-memory JSON
// document instead of a file, reusing the whole file-loading path. Group uses
// it to inject a member's slice of a combined configuration file.
func withBytesConfig(handler *sources.HandlerBytes) Option {
	return func(c *Structure) error {
		if handler == nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "withBytesConfig: handler must not be nil")
		}
		c.configHandler = handler
		return nil
	}
}

// withFlagNamePrefix prefixes the names this configuration registers on the
// flag set (eg "auth." -> --auth.port), leaving configuration keys, accessors,
// and file/env names untouched. Group uses it so members can share a flag set.
func withFlagNamePrefix(prefix string) Option {
	return func(c *Structure) error {
		c.flagNamePrefix = prefix
		return nil
	}
}

// WithName sets the name of the configuration
func WithName(name string) Option {
	return func(c *Structure) error {
		c.name = name
		return nil
	}
}

// WithFileConfig sets the config source/dest to a filename, and logs and error if unable to find file
func WithFileConfig(filename string) Option {
	return withFileConfig(filename, "WithFileConfig", false)
}

// WithDefaultFileConfig sets the config source/dest to a filename, and ignores if unable to find file
func WithDefaultFileConfig(filename string) Option {
	return withFileConfig(filename, "WithDefaultFileConfig", true)
}

func withFileConfig(filename string, funcName string, defaultConfig bool) Option {
	return func(c *Structure) error {
		if _, err := os.Stat(filename); err != nil {
			if defaultConfig {
				c.log().Debug("cfggo: optional configuration file unavailable", "filename", filename, "err", err)
				return nil
			}
			return c.WrapError(err, ErrCodeNotFound, "%s: configuration file %q is required but unavailable", funcName, filename)
		}
		if c.configHandler != nil {
			if defaultConfig {
				return nil
			}
			if !c.configHandler.IsDefault() {
				return c.WrapError(nil, ErrCodeInvalidArgument, "configHandler is already set, ignoring "+funcName)
			}
		}
		handler := sources.NewHandlerFile(filename, defaultConfig)
		c.configHandler = handler
		return nil
	}
}

// WithFileConfigParamName sets the config source/dest to a filename found by
// sniffing os.Args before cfggo's normal flag parsing runs.
//
// It recognizes -<argName>=filename, -<argName> filename and the same forms
// with two dashes, matching the standard flag package. The flag itself is
// registered together with cfggo's other flags, on whichever flag set is in
// use, so it may be combined with WithFlagSet / WithStandardFlags in either
// order. Because this is an early os.Args scan, it is best suited to simple
// bootstrap config flags. If your application already owns flag parsing,
// prefer parsing the config path yourself and passing it to WithFileConfig.
func WithFileConfigParamName(argName string) Option {
	filename := configPathFromArgs(os.Args[1:], argName)
	if filename == "" {
		GlobalLogger().Debug("cfggo: no filename found for config argument", "arg", argName)
		return func(c *Structure) error {
			c.configPathFlag = argName
			return nil
		}
	}
	wrap := withFileConfig(filename, "WithFileConfigParamName", false)
	return func(c *Structure) error {
		c.configPathFlag = argName
		if err := wrap(c); err != nil {
			c.log().Warn("cfggo: failed to apply file config", "filename", filename, "err", err)
			return err
		}
		return nil
	}
}

// configPathFromArgs returns the value given for the flag argName in args,
// accepting one or two leading dashes and both the "=value" and separate-token
// forms; "" when absent. Scanning stops at a "--" terminator
func configPathFromArgs(args []string, argName string) string {
	for i, arg := range args {
		if arg == "--" {
			return ""
		}
		if len(arg) < 2 || arg[0] != '-' || isNegativeNumber(arg) {
			continue
		}
		name := trimFlagDashes(arg)
		if value, found := strings.CutPrefix(name, argName+"="); found {
			return value
		}
		if name == argName && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// WithHTTPConfig sets the config source/dest to a filename
func WithHTTPConfig(httpLoader *http.Request, httpSaver *http.Request) Option {
	if httpLoader == nil && httpSaver == nil {
		return func(c *Structure) error {
			return c.WrapError(nil, ErrCodeInvalidArgument, "httpLoader and httpSaver cannot both be nil")
		}
	}
	return func(c *Structure) error {
		if c.configHandler != nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "configHandler is already set, ignoring WithHTTPConfig")
		}
		handler := sources.NewHandlerHTTP(httpLoader, httpSaver, false)
		c.configHandler = handler
		return nil
	}
}

// WithoutEnv skips cfggo's automatic environment variable override layer.
func WithoutEnv() Option {
	return func(c *Structure) error {
		c.skipEnv = true
		return nil
	}
}

// WithSkipEnvironment skips loading from environment variables.
//
// Deprecated: use WithoutEnv.
func WithSkipEnvironment() Option {
	return WithoutEnv()
}

// WithEnvPrefix makes cfggo's automatic environment-variable loader read only
// variables with prefix prepended to the usual env name. For example, with
// WithEnvPrefix("MYAPP_"), key "db.host" is read from MYAPP_DB_HOST instead of
// DB_HOST. An empty prefix preserves the default unprefixed mapping.
//
// This configures the normal environment override layer.
func WithEnvPrefix(prefix string) Option {
	return func(c *Structure) error {
		c.envPrefix = prefix
		c.envPrefixSet = true
		if _, ok := c.configHandler.(*sources.HandlerEnv); ok {
			c.configHandler = nil
		}
		return nil
	}
}

// WithAutoSave enables saving the configuration when ctx is cancelled.
//
// Unlike earlier versions, cfggo no longer installs a process-wide signal
// handler or calls os.Exit. The application owns its shutdown lifecycle and
// passes in a context — typically one derived from signal.NotifyContext:
//
//	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
//	defer stop()
//	cfg.Init(cfg, cfggo.WithFileConfig("config.json"), cfggo.WithAutoSave(ctx))
//
// When ctx is cancelled, any changed configuration is saved once via the
// configured ConfigHandler. For full control, call Save / SaveIfChanged
// directly instead. A nil ctx disables auto-save.
func WithAutoSave(ctx context.Context) Option {
	return func(c *Structure) error {
		c.autoSave = ctx != nil
		c.autoSaveCtx = ctx
		return nil
	}
}

// WithFlagSet registers cfggo's flags on an existing *flag.FlagSet (commonly
// flag.CommandLine) instead of creating a private one.
//
// In this mode cfggo does NOT parse the flag set during Init: the host owns the
// single canonical parse, e.g.
//
//	config.Init(config, cfggo.WithFlagSet(flag.CommandLine))
//	flag.Parse() // resolves cfggo flags AND any other library's flags together
//
// cfggo flag values still propagate automatically as the host parses, because
// each flag is backed by a flag.Value that writes straight into the config map.
//
// Because the host parses after Init returns, Init cannot see values supplied
// on the command line: validation failures for keys whose flag is present in
// os.Args are deferred, and the host should call Validate() once its Parse has
// completed. Each flag validates its own incoming value during Parse.
//
// When the configuration has secret-tagged keys, cfggo wraps the flag set's
// output so the flag package's own error text cannot echo a secret value; the
// error returned by a ContinueOnError Parse still can, so pass it through
// RedactFlagError before logging it.
func WithFlagSet(fs *flag.FlagSet) Option {
	return func(c *Structure) error {
		if fs == nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "WithFlagSet: flag set must not be nil")
		}
		c.flagSet = fs
		c.externalFlagSet = true
		return nil
	}
}

// WithStandardFlags registers cfggo's flags on the process-global
// flag.CommandLine, so a single flag.Parse() in the host resolves cfggo's flags
// alongside any flags registered by the standard library or other packages.
//
// It is a convenience wrapper around WithFlagSet(flag.CommandLine). As with
// WithFlagSet, cfggo does not parse during Init; the host must call
// flag.Parse() exactly once
func WithStandardFlags() Option {
	return WithFlagSet(flag.CommandLine)
}

// WithIgnoreUnknownVars makes cfggo ignore command-line flags it does not
// define instead of treating them as an error.
//
// By default an unrecognized flag makes Init return an error (wrapping
// ErrUnknownKey, with a "did you mean" hint when a close match exists).
// Enabling this option drops any unrecognized flags (and their separate
// values) before parsing, allowing cfggo to coexist with flags owned by other
// libraries or to tolerate typos.
//
// This affects only cfggo's own (private) flag parsing. When an external flag
// set is supplied via WithFlagSet/WithStandardFlags, the host owns parsing and
// is responsible for its own error-handling policy.
func WithIgnoreUnknownVars() Option {
	return func(c *Structure) error {
		c.ignoreUnknownVars = true
		return nil
	}
}

// WithSnakeCaseFieldNames controls how this configuration names untagged
// struct fields. When enabled, a field such as ServerPort is named
// "server_port"; when disabled, cfggo preserves the Go field name ("ServerPort").
// Explicit cfggo/cfg/config/json tags always take precedence.
func WithSnakeCaseFieldNames(enabled bool) Option {
	return func(c *Structure) error {
		c.snakeCaseFieldNames = enabled
		c.snakeCaseFieldNamesSet = true
		return nil
	}
}

// WithoutFlags disables command-line flag support entirely. cfggo will not
// create a private flag.FlagSet or register a flag.Value per configuration key,
// and will not parse os.Args during Init. Use it for programs that configure
// cfggo purely from files, environment variables, and struct defaults: it
// removes the flag system's setup cost from Init()
//
// It has no effect when an external flag set is supplied via
// WithFlagSet/WithStandardFlags, since that explicitly opts into flag support
// (the host owns parsing). A caller can still obtain a flag set later via
// GetFlagSet, which creates one lazily on demand
func WithoutFlags() Option {
	return func(c *Structure) error {
		c.noFlags = true
		return nil
	}
}

// WithValidation adds a validator for a configuration key
func WithValidation(key string, validator validcfg.Validator) Option {
	return func(c *Structure) error {
		c.RegisterValidator(key, validator)
		return nil
	}
}

// WithEnvConfig enables cfggo's automatic environment-variable loader with no
// prefix. It is equivalent to WithEnvPrefix("") and reads raw, un-prefixed
// variables such as PORT for key "port".
func WithEnvConfig() Option {
	return WithEnvPrefix("")
}

// WithConfigHandler uses the given ConfigHandler to save and load configuration.
func WithConfigHandler(handler sources.ConfigHandler) Option {
	return func(c *Structure) error {
		c.configHandler = handler
		return nil
	}
}

// WithLogger sets a custom logger for this configuration instance.
//
// During Init cfggo binds the configuration's name onto the logger as a
// "config" attribute when the logger supports it (the built-in DefaultLogger
// and a raw *slog.Logger both do), so every line is attributable to the
// instance. Custom loggers backed by fmt.Printf/log.Printf-style APIs should be
// wrapped with cfglogger.Plain so slog-style attrs are rendered into msg before
// the printf logger sees them.
//
// Note: SetLogLevel adjusts only the global logger.
// An instance logger supplied here controls its own verbosity;
// cfggo does not change its level
func WithLogger(logger cfglogger.Logger) Option {
	return func(c *Structure) error {
		c.logger = logger
		return nil
	}
}

// WithErrorWrapper sets a custom error wrapper for this configuration instance.
func WithErrorWrapper(wrapper cfgerror.Wrapper) Option {
	return func(c *Structure) error {
		c.errorWrapper = wrapper
		return nil
	}
}

// WithLenientLoad makes configuration load and validation failures during Init
// non-fatal: instead of returning an error, Init logs a warning and continues
// with whatever values it could resolve.
//
// By default cfggo is strict — a malformed configuration file (invalid JSON) or
// a value that fails a registered validator causes Init to return an error, so
// misconfiguration is surfaced loudly at startup rather than silently ignored.
// Use this option when you deliberately want best-effort loading (eg a
// long-running service that should start with defaults even if its config file
// is temporarily broken)
func WithLenientLoad() Option {
	return func(c *Structure) error {
		c.lenient = true
		return nil
	}
}

// WithIgnoreKeys exempts the given configuration keys from the "unrecognized
// key" diagnostics. Use it for keys that are intentionally present at runtime
// (eg command-line-only flags read elsewhere) but have no backing struct field,
// so they are not reported by Init, Diagnose, or Report
//
// Unlike the previous process-global IgnoreFlags, the exemptions are scoped to
// this configuration instance, so independent Structures never affect one
// another (and tests do not leak state between cases)
func WithIgnoreKeys(keys ...string) Option {
	return func(c *Structure) error {
		if c.ignoredKeys == nil {
			c.ignoredKeys = make(map[string]bool, len(keys))
		}
		for _, k := range keys {
			c.ignoredKeys[k] = true
		}
		return nil
	}
}

// WithStrictKeys makes Init return an error when a loaded configuration key has
// no matching struct field (typically a typo in a config file or environment
// variable). By default such keys are only logged as warnings.
func WithStrictKeys() Option {
	return func(c *Structure) error {
		c.strictKeys = true
		return nil
	}
}

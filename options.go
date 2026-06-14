package cfggo

import (
	"context"
	"flag"
	"net/http"
	"os"
	"strings"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
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

// WithName sets the name of the configuration
func WithName(name string) Option {
	return func(c *Structure) error {
		c.name = name
		return nil
	}
}

// withFileConfig sets the config source/dest to a filename, and logs and error if unable to find file
func WithFileConfig(filename string) Option {
	return withFileConfig(filename, "WithFileConfig", false)
}

// withFileConfig sets the config source/dest to a filename, and ignores if unable to find file
func WithDefaultFileConfig(filename string) Option {
	return withFileConfig(filename, "WithDefaultFileConfig", true)
}

func withFileConfig(filename string, funcName string, defaultConfig bool) Option {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			if !defaultConfig {
				GlobalLogger().Warn("cfggo: configuration file does not exist", "filename", filename)
			}
			return withNoop()
		}

		GlobalLogger().Warn("cfggo: error checking configuration file", "filename", filename, "err", err)
		return withNoop()
	}
	return func(c *Structure) error {
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

// WithFileConfigParamName sets the config source/dest to a filename defined in the command line arguments
func WithFileConfigParamName(argName string) Option {
	var filename string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		// Handle --config=filename.json format
		if strings.HasPrefix(arg, "--"+argName+"=") {
			filename = strings.TrimPrefix(arg, "--"+argName+"=")
			break
		}
		// Handle --config filename.json format
		if arg == "--"+argName && i+1 < len(os.Args) {
			filename = os.Args[i+1]
			break
		}
	}
	if filename == "" {
		GlobalLogger().Debug("cfggo: no filename found for config argument", "arg", argName)
		return func(c *Structure) error {
			c.ensureFlagSet()
			c.FlagSet.String(argName, "", "")
			return nil
		}
	}
	wrap := WithFileConfig(filename)
	return func(c *Structure) error {
		c.ensureFlagSet()
		c.FlagSet.String(argName, "", "")
		if err := wrap(c); err != nil {
			c.log().Warn("cfggo: failed to apply file config", "filename", filename, "err", err)
		}
		return nil
	}
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

// WithSkipEnvironment skips loading from environment variables
func WithSkipEnvironment() Option {
	return func(c *Structure) error {
		// Logger.Debug("Skipping environment variables")
		c.skipEnv = true
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
func WithFlagSet(fs *flag.FlagSet) Option {
	return func(c *Structure) error {
		if fs == nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "WithFlagSet: flag set must not be nil")
		}
		c.FlagSet = fs
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
// By default cfggo uses flag.ExitOnError for its private flag set, so an
// unrecognized flag prints usage and terminates the process. Enabling this
// option switches the private set to flag.ContinueOnError and drops any
// unrecognized flags (and their separate values) before parsing, allowing cfggo
// to coexist with flags owned by other libraries or to tolerate typos.
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

// WithEnvConfig sets a ConfigHandler that reads configuration from environment
// variables, optionally filtered to those whose names start with prefix
// (e.g. "MYAPP_"). An empty prefix accepts all environment variables.
// The handler is read-only: saving configuration via env vars is a no-op.
func WithEnvConfig(prefix string) Option {
	return func(c *Structure) error {
		if c.configHandler != nil {
			return c.WrapError(nil, ErrCodeInvalidArgument, "configHandler is already set, ignoring WithEnvConfig")
		}
		c.configHandler = sources.NewHandlerEnv(prefix, true)
		return nil
	}
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
// instance
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

// WithErrorWrapper sets a custom error wrapper for this configuration instance
func WithErrorWrapper(wrapper errwrapper.ErrorWrapper) Option {
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

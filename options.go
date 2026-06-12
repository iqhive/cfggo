package cfggo

import (
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
				Logger.Warn("filename (" + filename + ") does not exist")
			}
			return withNoop()
		}

		Logger.Warn("error loading filename (" + filename + "): " + err.Error())
		return withNoop()
	}
	return func(c *Structure) error {
		if c.configHandler != nil {
			if defaultConfig {
				return nil
			}
			if !c.configHandler.IsDefault() {
				return c.WrapError(nil, 400, "configHandler is already set, ignoring "+funcName)
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
		Logger.Debug("no filename found for argument (" + argName + ")")
		return func(c *Structure) error {
			c.FlagSet.String(argName, "", "")
			return nil
		}
	}
	wrap := WithFileConfig(filename)
	return func(c *Structure) error {
		c.FlagSet.String(argName, "", "")
		if err := wrap(c); err != nil {
			Logger.Warnf("Failed to apply WithFileConfig: %v", err)
		}
		return nil
	}
}

// WithHTTPConfig sets the config source/dest to a filename
func WithHTTPConfig(httpLoader *http.Request, httpSaver *http.Request) Option {
	if httpLoader == nil && httpSaver == nil {
		return func(c *Structure) error {
			return c.WrapError(nil, 400, "httpLoader and httpSaver cannot both be nil")
		}
	}
	return func(c *Structure) error {
		if c.configHandler != nil {
			return c.WrapError(nil, 400, "configHandler is already set, ignoring WithHTTPConfig")
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

// WithAutoSave enables automatic saving of configuration on program exit
func WithAutoSave() Option {
	return func(c *Structure) error {
		c.autoSave = true
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
			return c.WrapError(nil, 400, "WithFlagSet: flag set must not be nil")
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
			return c.WrapError(nil, 400, "configHandler is already set, ignoring WithEnvConfig")
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

// WithLogger sets a custom logger for this configuration instance
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

// WithErrorWrapperWithLogger sets a custom error wrapper with logging for this configuration instance
func WithErrorWrapperWithLogger(wrapper errwrapper.ErrorWrapperWithLogger) Option {
	return func(c *Structure) error {
		c.errorWrapperWithLogger = wrapper
		return nil
	}
}

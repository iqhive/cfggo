package cfggo

import (
	"flag"
	"net/http"
	"os"
	"strings"
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
				return ErrorWrapper(nil, 400, "configHandler is already set, ignoring "+funcName)
			}
		}
		handler := &handlerFile{filename: filename, defaultConfig: defaultConfig}
		c.configHandler = handler
		return nil
	}
}

// WithFileConfigParamName sets the config source/dest to a filename defined in the command line arguments
func WithFileConfigParamName(argName string) Option {
	flag.String(argName, "", "config file")
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
		Logger.Warn("no filename found for argument (" + argName + ")")
		return withNoop()
	}
	return WithFileConfig(filename)
}

// WithHTTPConfig sets the config source/dest to a filename
func WithHTTPConfig(httpLoader *http.Request, httpSaver *http.Request) Option {
	if httpLoader == nil && httpSaver == nil {
		return func(c *Structure) error {
			return ErrorWrapper(nil, 400, "httpLoader and httpSaver cannot both be nil")
		}
	}
	return func(c *Structure) error {
		if c.configHandler != nil {
			return ErrorWrapper(nil, 400, "configHandler is already set, ignoring WithHTTPConfig")
		}
		handler := &handlerHTTP{}
		if httpLoader != nil {
			handler.source = *httpLoader
		}
		if httpSaver != nil {
			handler.dest = *httpSaver
		}
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

// WithFlagSet sets a custom FlagSet for the configuration
func WithFlagSet(fs *flag.FlagSet) Option {
	return func(c *Structure) error {
		c.FlagSet = fs
		return nil
	}
}

// WithValidation adds a validator for a configuration key
func WithValidation(key string, validator Validator) Option {
	return func(c *Structure) error {
		c.RegisterValidator(key, validator)
		return nil
	}
}

// WithConfigHandler uses the given ConfigHandler to save and load configuration.
func WithConfigHandler(handler ConfigHandler) Option {
	return func(c *Structure) error {
		c.configHandler = handler
		return nil
	}
}

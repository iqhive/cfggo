package cfggo

import (
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

// WithFileConfig sets the config source/dest to a filename
func WithFileConfig(filename string) Option {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		Logger.Warn("filename %s does not exist", filename)
	}
	return func(c *Structure) error {
		if c.configHandler != nil {
			return ErrorWrapper(nil, 400, "configHandler is already set, ignoring WithFileConfig")
		}
		handler := &handlerFile{filename: filename}
		c.configHandler = handler
		return nil
	}
}

// WithFileConfigParamName sets the config source/dest to a filename defined in the command line arguments
func WithFileConfigParamName(argName string) Option {
	var fname string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		// Handle --config=filename.json format
		if strings.HasPrefix(arg, argName+"=") {
			fname = strings.TrimPrefix(arg, argName+"=")
			break
		}
		// Handle --config filename.json format
		if arg == argName && i+1 < len(os.Args) {
			fname = os.Args[i+1]
			break
		}
	}
	if fname == "" {
		// Logger.Warn("no filename found for argument %s", argName)
		return withNoop()
	}
	return WithFileConfig(fname)
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

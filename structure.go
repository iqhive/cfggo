package cfggo

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
	"github.com/iqhive/cfggo/internal/env"
	"github.com/iqhive/cfggo/sources"
	"github.com/iqhive/cfggo/validcfg"
)

var internalEnvLoader = env.NewLoader()

// Structure is the type that configuration structs must embed.
// All lifecycle methods (Init, InitSelf, InitMyParent) live here.
type Structure struct {
	name               string
	configHandler      sources.ConfigHandler
	skipEnv            bool
	changed            bool
	defaultsAlreadySet bool
	parent             interface{}
	configData         map[string]interface{}
	autoSave           bool
	autoSaveCtx        context.Context

	FlagSet *flag.FlagSet
	// externalFlagSet is true when the caller supplied the FlagSet (e.g. via
	// WithFlagSet / WithStandardFlags). In that mode cfggo registers its flags
	// on the supplied set but does NOT parse it: the host owns the single
	// canonical Parse() call, commonly flag.Parse()
	externalFlagSet bool
	// ignoreUnknownVars, when true, makes cfggo ignore command-line flags it
	// doesn't define instead of treating them as an error. Enabled via
	// WithIgnoreUnknownVars. The default (false) preserves the historical
	// behaviour of exiting on unknown flags
	ignoreUnknownVars bool

	logger                 cfglogger.Logger
	errorWrapper           errwrapper.ErrorWrapper
	errorWrapperWithLogger errwrapper.ErrorWrapperWithLogger

	// configMutex guards this instance's configData, provenance, and flag/func
	// wiring. It is per-instance so independent Structure values never contend
	// on a single shared lock.
	configMutex sync.RWMutex

	// provenance records, per key, where the current value came from. Guarded
	// by configMutex.
	provenance map[string]Source

	validationMap   map[string]map[string]validcfg.Validator
	validationMutex sync.RWMutex

	// changeCallbacks holds OnChange listeners, guarded by callbackMutex.
	callbackMutex   sync.Mutex
	changeCallbacks []changeCallback
	nextCallbackID  int
}

// DefaultValue returns a function that always returns x, satisfying the
// func()-returning field pattern used by cfggo config structs.
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

// Init initialises the configuration and returns an error on failure
// parent must be a pointer to the struct that embeds Structure
func (c *Structure) Init(parent interface{}, options ...Option) error {
	if c.FlagSet == nil {
		// Default behaviour: an unrecognized flag terminates the process with
		// usage output (flag.ExitOnError). Use WithIgnoreUnknownVars to instead
		// ignore flags cfggo doesn't define.
		c.FlagSet = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		c.FlagSet.Usage = func() {
			fmt.Fprintf(c.FlagSet.Output(), "Usage of %s:\n", os.Args[0])
			c.FlagSet.PrintDefaults()
		}
	}

	if c.validationMap == nil {
		c.validationMap = make(map[string]map[string]validcfg.Validator)
	}

	if c.logger == nil {
		c.logger = Logger
	}
	if c.errorWrapper == nil {
		c.errorWrapper = ErrorWrapper
	}
	if c.errorWrapperWithLogger == nil {
		c.errorWrapperWithLogger = errwrapper.NewDefaultErrorWrapperWithLogger()
	}

	v := reflect.ValueOf(parent)
	if v.Kind() != reflect.Ptr {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		parent = ptr.Interface()
		c.log().Warn("Structure: Init() must be called with a parent struct pointer, not a struct")
	} else if v.Type().Elem().Kind() == reflect.Ptr {
		return c.WrapError(nil, 400, "Init: parent must be a pointer to a struct, not a pointer to a pointer")
	}

	if c.parent != nil {
		c.log().Warn("Structure: Init() called more than once")
		return nil
	}
	c.parent = parent

	for _, option := range options {
		if err := option(c); err != nil {
			return c.WrapError(err, 0, "Init: option returned error")
		}
	}

	if c.name == "" {
		c.name = reflect.TypeOf(c.parent).Elem().Name()
	}

	c.setupConfigData()
	c.setDefaultsFromTags()
	c.replaceConfigFuncs()

	if c.configHandler != nil {
		if err := c.loadConfig(false); err != nil {
			c.logErrorf("LoadConfig: %v", err)
			// A non-default source that fails to load is fatal to Init; a
			// default source (WithDefaultFileConfig) is allowed to be absent.
			if !c.configHandler.IsDefault() {
				return err
			}
		}
	}

	c.loadFromEnv()
	c.createFlags()
	// When the caller supplied the flag set (e.g. flag.CommandLine), they own the
	// single canonical Parse() call so cfggo flags resolve together with any
	// other library's flags. Otherwise cfggo parses its own private set here
	if !c.externalFlagSet {
		c.parseFlags()
	}

	if err := c.Validate(); err != nil {
		c.logWarnf("Configuration validation failed: %v", err)
	}

	c.startAutoSave()
	return nil
}

// WrapError wraps an error using the instance's error wrapper.
func (c *Structure) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	if c.errorWrapper == nil {
		c.errorWrapper = ErrorWrapper
	}
	return c.errorWrapper(err, errorcode, msg, args...)
}

// WrapErrorWithLogging wraps an error and logs it using the instance's logger.
//
// Deprecated: prefer WrapError and log the returned error yourself (e.g.
// c.GetLogger().Error(err.Error())). Having two wrapping entry points is a
// frequent source of "which do I use?" confusion; this variant will be removed
// in a future version.
func (c *Structure) WrapErrorWithLogging(err error, errorcode int, msg string, args ...interface{}) error {
	if c.errorWrapperWithLogger == nil {
		c.errorWrapperWithLogger = errwrapper.NewDefaultErrorWrapperWithLogger()
	}
	if c.logger == nil {
		c.logger = Logger
	}
	return c.errorWrapperWithLogger(c.logger, err, errorcode, msg, args...)
}

// SetLogger sets the instance-specific logger.
func (c *Structure) SetLogger(logger cfglogger.Logger) {
	c.logger = logger
}

// SetErrorWrapper sets the instance-specific error wrapper.
func (c *Structure) SetErrorWrapper(wrapper errwrapper.ErrorWrapper) {
	c.errorWrapper = wrapper
}

// SetErrorWrapperWithLogger sets the instance-specific error wrapper with logging.
func (c *Structure) SetErrorWrapperWithLogger(wrapper errwrapper.ErrorWrapperWithLogger) {
	c.errorWrapperWithLogger = wrapper
}

// GetLogger returns the instance's logger, falling back to the global Logger.
func (c *Structure) GetLogger() cfglogger.Logger {
	if c.logger == nil {
		return Logger
	}
	return c.logger
}

// InitSelf is a convenience variant of Init for when the struct initialises
// itself (i.e. parent == c).
func (c *Structure) InitSelf(options ...Option) error {
	return c.Init(c, options...)
}

// ensureInit lazily initialises the configuration via InitSelf when a method is
// called before Init. The initialisation error is logged because the calling
// method has no way to return it; call Init explicitly to handle errors.
func (c *Structure) ensureInit() {
	if c.parent == nil {
		if err := c.InitSelf(); err != nil {
			c.log().Error("cfggo: lazy initialisation failed: " + err.Error())
		}
	}
}

// ReloadConfig reloads the configuration from all sources.
func (c *Structure) ReloadConfig() error {
	c.ensureInit()
	c.logInfof("ReloadConfig %s", c.name)
	return c.Reload()
}

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
// All lifecycle methods (Init, InitSelf) live here
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

	// lenient, when true (via WithLenientLoad), downgrades configuration load
	// and validation failures during Init from hard errors to logged warnings.
	// The default (false) makes Init() return these errors so misconfiguration is
	// surfaced loudly instead of silently ignored
	lenient bool

	// strictKeys, when true (via WithStrictKeys), makes Init() return an error
	// when a loaded configuration key is not backed by a struct field. The
	// default (false) only logs a warning for such keys
	strictKeys bool

	logger       cfglogger.Logger
	errorWrapper errwrapper.ErrorWrapper

	// fields holds precomputed metadata for every leaf (non-struct) config
	// field, keyed by its dotted config key. It is built once during Init so
	// help-tag lookups, the config reference, and key-recognition checks do not
	// repeatedly walk the parent struct via reflection. ignoredFields records
	// the dotted keys of fields tagged `cfggo:"-"`
	fields        map[string]fieldInfo
	ignoredFields map[string]bool

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
		c.logger = GlobalLogger()
	}
	if c.errorWrapper == nil {
		c.errorWrapper = GlobalErrorWrapper()
	}

	v := reflect.ValueOf(parent)
	if v.Kind() != reflect.Ptr {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		parent = ptr.Interface()
		c.log().Warn("Structure: Init() must be called with a parent struct pointer, not a struct")
	} else if v.Type().Elem().Kind() == reflect.Ptr {
		return c.WrapError(nil, ErrCodeInvalidArgument, "Init: parent must be a pointer to a struct, not a pointer to a pointer")
	}

	if c.parent != nil {
		c.log().Warn("Structure: Init() called more than once")
		return nil
	}
	c.parent = parent

	// Establish the default name before running options. Options such as
	// WithValidation register validators keyed by c.name, so the name must be
	// settled first or those validators would be stored under "" and never run
	// (WithName, if supplied, still overrides this default during the loop)
	if c.name == "" {
		c.name = reflect.TypeOf(c.parent).Elem().Name()
	}

	for _, option := range options {
		if err := option(c); err != nil {
			return c.WrapError(err, ErrCodeNone, "Init: option returned error")
		}
	}

	c.buildFieldMeta()
	c.setupConfigData()
	c.setDefaultsFromTags()
	c.replaceConfigFuncs()

	if c.configHandler != nil {
		if err := c.loadConfig(false); err != nil {
			// A non-default source that fails to load is fatal to Init; a
			// default source (WithDefaultFileConfig) is allowed to be absent.
			if !c.configHandler.IsDefault() {
				c.log().Error("cfggo: failed to load configuration source", "err", err)
				return err
			}
			c.log().Warn("cfggo: optional configuration source could not be loaded", "err", err)
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

	// Surface configuration keys that are not backed by a struct field. These
	// are almost always typos (e.g. "portt" in a JSON file) that would
	// otherwise be silently ignored. WithStrictKeys upgrades this to an error
	if unrecognized := c.unrecognizedKeys(); len(unrecognized) > 0 {
		for _, key := range unrecognized {
			c.log().Warn("cfggo: unrecognized configuration key (no matching struct field)", "config", c.name, "key", key)
		}
		if c.strictKeys {
			return c.WrapError(wrapKind(ErrUnknownKey, fmt.Errorf("%v", unrecognized)), ErrCodeNotFound,
				"unrecognized configuration keys")
		}
	}

	if err := c.Validate(); err != nil {
		if !c.lenient {
			c.log().Error("cfggo: configuration validation failed", "err", err)
			return err
		}
		c.log().Warn("cfggo: configuration validation failed", "err", err)
	}

	c.startAutoSave()
	return nil
}

// WrapError wraps an error using the instance's error wrapper.
func (c *Structure) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	if c.errorWrapper == nil {
		c.errorWrapper = GlobalErrorWrapper()
	}
	return c.errorWrapper(err, errorcode, msg, args...)
}

// SetLogger sets the instance-specific logger.
func (c *Structure) SetLogger(logger cfglogger.Logger) {
	c.logger = logger
}

// SetErrorWrapper sets the instance-specific error wrapper.
func (c *Structure) SetErrorWrapper(wrapper errwrapper.ErrorWrapper) {
	c.errorWrapper = wrapper
}

// GetLogger returns the instance's logger, falling back to the global logger
func (c *Structure) GetLogger() cfglogger.Logger {
	if c.logger == nil {
		return GlobalLogger()
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
	c.log().Info("cfggo: reloading configuration", "config", c.name)
	return c.Reload()
}

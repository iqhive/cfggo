package cfggo

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"sync"
	"unsafe"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
	"github.com/iqhive/cfggo/internal/env"
	"github.com/iqhive/cfggo/sources"
	"github.com/iqhive/cfggo/validcfg"
)

var configMutex sync.RWMutex

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

	FlagSet *flag.FlagSet

	boolCallbacks map[string]func(bool)

	logger                 cfglogger.Logger
	errorWrapper           errwrapper.ErrorWrapper
	errorWrapperWithLogger errwrapper.ErrorWrapperWithLogger

	validationMap   map[string]map[string]validcfg.Validator
	validationMutex sync.RWMutex
}

// DefaultValue returns a function that always returns x, satisfying the
// func()-returning field pattern used by cfggo config structs.
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

// Init initialises the configuration. parent must be a pointer to the struct
// that embeds Structure.
func (c *Structure) Init(parent interface{}, options ...Option) {
	if c.FlagSet == nil {
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
		Logger.Warn("Structure: Init() must be called with a parent struct pointer, not a struct")
	} else {
		if v.Type().Elem().Kind() == reflect.Ptr {
			Logger.Error("Structure: Init() parent must not be a pointer to a pointer")
			os.Exit(1)
		}
	}

	if c.parent != nil {
		Logger.Warn("Structure: Init() called more than once")
		return
	}
	c.parent = parent

	for _, option := range options {
		err := option(c)
		if err != nil {
			Logger.Errorf("Structure: Init() option returned error: %v", err)
			os.Exit(1)
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
			Logger.Errorf("LoadConfig: %v", err)
			if c.configHandler != nil && !c.configHandler.IsDefault() {
				os.Exit(1)
			}
		}
	}

	c.loadFromEnv()
	c.createFlags()
	c.parseFlags()

	if err := c.Validate(); err != nil {
		Logger.Warnf("Configuration validation failed: %v", err)
	}
}

// WrapError wraps an error using the instance's error wrapper.
func (c *Structure) WrapError(err error, errorcode int, msg string, args ...interface{}) error {
	if c.errorWrapper == nil {
		c.errorWrapper = ErrorWrapper
	}
	return c.errorWrapper(err, errorcode, msg, args...)
}

// WrapErrorWithLogging wraps an error and logs it using the instance's logger.
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
func (c *Structure) InitSelf(options ...Option) {
	c.Init(c, options...)
}

// InitMyParent discovers the enclosing struct via unsafe pointer arithmetic and
// calls Init with it. The embedded Structure field must be anonymous.
func (c *Structure) InitMyParent(options ...Option) {
	structPtr := unsafe.Pointer(reflect.ValueOf(c).Pointer())

	parentValue := reflect.ValueOf(c).Elem().Field(0)
	parentType := parentValue.Type().Elem()

	for i := 0; i < parentType.NumField(); i++ {
		field := parentType.Field(i)
		if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
			offset := field.Offset
			parentPtr := unsafe.Pointer(uintptr(structPtr) - offset)
			parent := reflect.NewAt(parentType, parentPtr).Interface()
			c.Init(parent, options...)
			return
		}
	}

	Logger.Error("InitNew(): Could not determine parent struct automatically")
	os.Exit(1)
}

// ReloadConfig reloads the configuration from all sources.
func (c *Structure) ReloadConfig() error {
	if c.parent == nil {
		c.InitSelf()
	}
	Logger.Infof("ReloadConfig %s", c.name)
	return c.Reload()
}

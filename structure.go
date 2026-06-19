package cfggo

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/internal/env"
	"github.com/iqhive/cfggo/sources"
	"github.com/iqhive/cfggo/validcfg"
)

var internalEnvLoader = env.NewLoader()

// DefaultSnakeCaseFieldNames controls how untagged struct fields are named by
// default. When false, cfggo preserves its historical behavior and uses the Go
// field name as-is (ServerPort -> "ServerPort"). When true, untagged fields use
// snake_case (ServerPort -> "server_port"). Per-config options override this.
var DefaultSnakeCaseFieldNames bool

// Structure is the type that configuration structs must embed.
// All lifecycle methods (Init, InitSelf) live here
type Structure struct {
	name          string
	configHandler sources.ConfigHandler
	skipEnv       bool
	envPrefix     string
	envPrefixSet  bool
	changed       bool
	changeVersion uint64
	parent        interface{}
	configData    map[string]interface{}
	defaultData   map[string]interface{}
	autoSave      bool
	autoSaveCtx   context.Context

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
	// noFlags, when true (via WithoutFlags), skips command-line flag
	// registration and parsing entirely. Programs that configure cfggo purely
	// from files/env/defaults avoid building a flag.FlagSet and a flag.Value
	// per key. It is mutually exclusive with WithFlagSet/WithStandardFlags
	noFlags bool

	// lenient, when true (via WithLenientLoad), downgrades configuration load
	// and validation failures during Init from hard errors to logged warnings.
	// The default (false) makes Init() return these errors so misconfiguration is
	// surfaced loudly instead of silently ignored
	lenient bool

	// strictKeys, when true (via WithStrictKeys), makes Init() return an error
	// when a loaded configuration key is not backed by a struct field. The
	// default (false) only logs a warning for such keys
	strictKeys bool

	// snakeCaseFieldNames controls the fallback name for fields that do not have
	// cfggo/cfg/config/json tags. The Set flag lets WithSnakeCaseFieldNames
	// override DefaultSnakeCaseFieldNames for this instance.
	snakeCaseFieldNames    bool
	snakeCaseFieldNamesSet bool

	logger       cfglogger.Logger
	errorWrapper cfgerror.Wrapper

	// plan is the cached, reflection-free blueprint for the parent struct's
	// type: every leaf field's dotted key, help/default tags, accessor type,
	// and field-index path. It is computed once per Go type and shared across
	// all Structure instances of that type, so repeated Init calls (and
	// help-tag lookups, the config reference, key-recognition checks) do no
	// per-field tag parsing or StructField copying.
	plan *structPlan

	// configMutex guards this instance's configData, provenance, and flag/func
	// wiring. It is per-instance so independent Structure values never contend
	// on a single shared lock.
	configMutex sync.RWMutex

	// provenance records, per key, where the current value came from. Guarded
	// by configMutex.
	provenance map[string]Source

	// provenanceTrail records, per key, the ordered chain of sources that have
	// contributed to the value (eg default -> file -> env). It is populated
	// lazily: a key that is only ever set by a single source allocates no trail
	// entry, so the common case stays allocation-free. Guarded by configMutex
	provenanceTrail map[string][]Source

	// ignoredKeys holds configuration keys that should be exempt from the
	// "unrecognized key" diagnostics (eg command-line-only flags). Populated via
	// WithIgnoreKeys. It replaces the previous process-global IgnoreFlags so the
	// exemptions are scoped to this instance
	ignoredKeys map[string]bool

	// lazyInitWarn ensures the "lazy initialisation" warning emitted by
	// ensureInit is logged at most once per instance
	lazyInitWarn sync.Once
	initMutex    sync.Mutex
	initializing atomic.Bool
	initialized  atomic.Bool

	// validationMap holds the registered validators keyed by config key. It is
	// per-instance (a Structure has exactly one configuration), so it is a flat
	// key->validator map rather than being nested under the config name
	validationMap   map[string]validcfg.Validator
	validationMutex sync.RWMutex

	// changeCallbacks holds OnChange listeners, guarded by callbackMutex.
	callbackMutex   sync.Mutex
	changeCallbacks []changeCallback
	nextCallbackID  int
}

// DefaultValue returns a function that returns x, satisfying the func()-returning
// field pattern used by cfggo config structs.
//
// Mutable defaults (maps, slices, and pointers) are cloned on each call so a
// pre-Init accessor read cannot mutate the caller's original default value or
// poison the value later captured during Init.
func DefaultValue[T any](x T) func() T {
	return DefaultClone(x)
}

// DefaultClone returns a default accessor that clones mutable values before
// returning them. It is useful when you want to make the cloning behavior
// explicit at the field declaration site.
func DefaultClone[T any](x T) func() T {
	return func() T {
		return cloneDefaultValue(x)
	}
}

func cloneDefaultValue[T any](x T) T {
	v := reflect.ValueOf(x)
	if !v.IsValid() {
		return x
	}
	cloned := cloneMutableReflectValue(v)
	if !cloned.IsValid() {
		return x
	}
	out, ok := cloned.Interface().(T)
	if !ok {
		return x
	}
	return out
}

// Init initialises the configuration and returns an error on failure
// parent must be a pointer to the struct that embeds Structure
func (c *Structure) Init(parent interface{}, options ...Option) error {
	c.initMutex.Lock()
	defer c.initMutex.Unlock()
	if c.initialized.Load() {
		c.log().Warn("Structure: Init() called more than once")
		return c.WrapError(ErrAlreadyInitialized, ErrCodeInvalidArgument, "Init: configuration already initialized")
	}
	snapshot := c.snapshotInitState()
	c.initializing.Store(true)
	defer c.initializing.Store(false)
	if err := c.initLocked(parent, options...); err != nil {
		c.restoreInitState(snapshot)
		return err
	}
	c.initialized.Store(true)
	return nil
}

type initStateSnapshot struct {
	name              string
	configHandler     sources.ConfigHandler
	skipEnv           bool
	envPrefix         string
	envPrefixSet      bool
	changed           bool
	changeVersion     uint64
	parent            interface{}
	configData        map[string]interface{}
	defaultData       map[string]interface{}
	autoSave          bool
	autoSaveCtx       context.Context
	FlagSet           *flag.FlagSet
	externalFlagSet   bool
	ignoreUnknownVars bool
	noFlags           bool
	lenient           bool
	strictKeys        bool
	snakeCaseNames    bool
	snakeCaseNamesSet bool
	logger            cfglogger.Logger
	errorWrapper      cfgerror.Wrapper
	plan              *structPlan
	provenance        map[string]Source
	provenanceTrail   map[string][]Source
	ignoredKeys       map[string]bool
	validationMap     map[string]validcfg.Validator
}

func (c *Structure) snapshotInitState() initStateSnapshot {
	c.configMutex.RLock()
	configData := cloneInterfaceMap(c.configData)
	defaultData := cloneInterfaceMap(c.defaultData)
	provenance := cloneSourceMap(c.provenance)
	provenanceTrail := cloneSourceTrailMap(c.provenanceTrail)
	c.configMutex.RUnlock()

	c.validationMutex.RLock()
	validationMap := cloneValidatorMap(c.validationMap)
	c.validationMutex.RUnlock()

	return initStateSnapshot{
		name:              c.name,
		configHandler:     c.configHandler,
		skipEnv:           c.skipEnv,
		envPrefix:         c.envPrefix,
		envPrefixSet:      c.envPrefixSet,
		changed:           c.changed,
		changeVersion:     c.changeVersion,
		parent:            c.parent,
		configData:        configData,
		defaultData:       defaultData,
		autoSave:          c.autoSave,
		autoSaveCtx:       c.autoSaveCtx,
		FlagSet:           c.FlagSet,
		externalFlagSet:   c.externalFlagSet,
		ignoreUnknownVars: c.ignoreUnknownVars,
		noFlags:           c.noFlags,
		lenient:           c.lenient,
		strictKeys:        c.strictKeys,
		snakeCaseNames:    c.snakeCaseFieldNames,
		snakeCaseNamesSet: c.snakeCaseFieldNamesSet,
		logger:            c.logger,
		errorWrapper:      c.errorWrapper,
		plan:              c.plan,
		provenance:        provenance,
		provenanceTrail:   provenanceTrail,
		ignoredKeys:       cloneBoolMap(c.ignoredKeys),
		validationMap:     validationMap,
	}
}

func (c *Structure) restoreInitState(snapshot initStateSnapshot) {
	planBuilt := c.plan != nil
	c.name = snapshot.name
	c.configHandler = snapshot.configHandler
	c.skipEnv = snapshot.skipEnv
	c.envPrefix = snapshot.envPrefix
	c.envPrefixSet = snapshot.envPrefixSet
	c.autoSave = snapshot.autoSave
	c.autoSaveCtx = snapshot.autoSaveCtx
	c.FlagSet = snapshot.FlagSet
	c.externalFlagSet = snapshot.externalFlagSet
	c.ignoreUnknownVars = snapshot.ignoreUnknownVars
	c.noFlags = snapshot.noFlags
	c.lenient = snapshot.lenient
	c.strictKeys = snapshot.strictKeys
	c.snakeCaseFieldNames = snapshot.snakeCaseNames
	c.snakeCaseFieldNamesSet = snapshot.snakeCaseNamesSet
	c.logger = snapshot.logger
	c.errorWrapper = snapshot.errorWrapper
	if !planBuilt {
		c.plan = snapshot.plan
		c.parent = snapshot.parent
	}
	c.initialized.Store(false)

	c.configMutex.Lock()
	if !planBuilt {
		c.changed = snapshot.changed
		c.changeVersion = snapshot.changeVersion
		c.configData = snapshot.configData
		c.defaultData = snapshot.defaultData
		c.provenance = snapshot.provenance
		c.provenanceTrail = snapshot.provenanceTrail
	}
	c.ignoredKeys = snapshot.ignoredKeys
	c.configMutex.Unlock()

	c.validationMutex.Lock()
	c.validationMap = snapshot.validationMap
	c.validationMutex.Unlock()
}

func cloneInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = cloneMutableInterface(v)
	}
	return out
}

func cloneMutableInterface(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return cloneMutableReflectValue(reflect.ValueOf(v)).Interface()
}

func cloneSourceMap(in map[string]Source) map[string]Source {
	if in == nil {
		return nil
	}
	out := make(map[string]Source, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSourceTrailMap(in map[string][]Source) map[string][]Source {
	if in == nil {
		return nil
	}
	out := make(map[string][]Source, len(in))
	for k, v := range in {
		cp := make([]Source, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneValidatorMap(in map[string]validcfg.Validator) map[string]validcfg.Validator {
	if in == nil {
		return nil
	}
	out := make(map[string]validcfg.Validator, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (c *Structure) useSnakeCaseFieldNames() bool {
	if c.snakeCaseFieldNamesSet {
		return c.snakeCaseFieldNames
	}
	return DefaultSnakeCaseFieldNames
}

func (c *Structure) initLocked(parent interface{}, options ...Option) error {
	if c.validationMap == nil {
		c.validationMap = make(map[string]validcfg.Validator)
	}

	if c.logger == nil {
		c.logger = GlobalLogger()
	}
	if c.errorWrapper == nil {
		c.errorWrapper = GlobalErrorWrapper()
	}

	if parent == nil {
		return c.WrapError(nil, ErrCodeInvalidArgument, "Init: parent must not be nil")
	}
	v := reflect.ValueOf(parent)
	if v.Kind() != reflect.Ptr {
		return c.WrapError(nil, ErrCodeInvalidArgument, "Init: parent must be a pointer to a struct, got %s", v.Kind())
	}
	if v.Type().Elem().Kind() == reflect.Ptr {
		return c.WrapError(nil, ErrCodeInvalidArgument, "Init: parent must be a pointer to a struct, not a pointer to a pointer")
	}

	if c.initialized.Load() {
		c.log().Warn("Structure: Init() called more than once")
		return c.WrapError(ErrAlreadyInitialized, ErrCodeInvalidArgument, "Init: configuration already initialized")
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
			return c.WrapError(err, ErrorCode(err), "Init: option returned error")
		}
	}

	// Bind the (now-final) configuration name onto the instance logger so every
	// line this Structure emits is attributable to it without each call site
	// repeating the name. No-op for loggers that cannot attach attributes
	c.logger = bindConfigName(c.log(), c.name)

	parentType := reflect.TypeOf(c.parent)
	for parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}
	c.plan = planForType(parentType, c.useSnakeCaseFieldNames())
	for _, s := range c.plan.suspects {
		c.log().Warn("cfggo: field has a cfggo tag but is not a func() T accessor; "+
			"it will be ignored (did you mean func() "+s.Type+"?)", "key", s.Key, "type", s.Type)
	}
	if err := c.applyPlan(); err != nil {
		return err
	}

	if err := c.validateConfigShape(); err != nil {
		return err
	}

	if c.configHandler != nil {
		if err := c.loadConfig(false); err != nil {
			// A non-default source that fails to load is fatal to Init; a
			// default source (WithDefaultFileConfig) is allowed to be absent.
			if !c.configHandler.IsDefault() {
				c.log().Error("cfggo: Init failed", "err", err)
				return err
			}
			c.log().Warn("cfggo: optional configuration source could not be loaded", "err", err)
		}
	}

	if err := c.loadFromEnv(); err != nil {
		if !c.lenient {
			c.log().Error("cfggo: environment configuration failed", "err", err)
			return err
		}
		c.log().Warn("cfggo: environment configuration failed", "err", err)
	}

	// Flag handling. When the caller supplied the flag set (e.g.
	// flag.CommandLine) they own the single canonical Parse() call, so cfggo
	// only registers its flags and lets the host parse. Otherwise cfggo creates
	// a private set, registers, and parses it here. WithoutFlags skips the
	// private flag set entirely (no FlagSet allocation, no per-key flag.Value)
	// for programs that configure purely from files, env, and defaults.
	if c.externalFlagSet {
		c.createFlags()
	} else if !c.noFlags {
		if c.FlagSet == nil {
			// Default behaviour: an unrecognized flag terminates the process
			// with usage output (flag.ExitOnError). Use WithIgnoreUnknownVars to
			// instead ignore flags cfggo doesn't define.
			c.FlagSet = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			c.FlagSet.Usage = func() {
				fmt.Fprintf(c.FlagSet.Output(), "Usage of %s:\n", os.Args[0])
				c.FlagSet.PrintDefaults()
			}
		}
		c.createFlags()
		if err := c.parseFlags(); err != nil {
			return err
		}
	}

	if err := c.checkUnrecognizedKeys(); err != nil {
		return err
	}

	if err := c.validate(); err != nil {
		if !c.lenient {
			c.log().Error("cfggo: configuration validation failed", "err", err)
			return err
		}
		c.log().Warn("cfggo: configuration validation failed", "err", err)
	}

	c.startAutoSave()
	return nil
}

// checkUnrecognizedKeys surfaces configuration keys that are not backed by a
// struct field. These are almost always typos (e.g. "portt" in a JSON file)
// that would otherwise be silently ignored. WithStrictKeys upgrades this to an
// error.
func (c *Structure) checkUnrecognizedKeys() error {
	if unrecognized := c.unrecognizedKeys(); len(unrecognized) > 0 {
		for _, key := range unrecognized {
			attrs := []any{"key", key}
			if suggestion := c.suggestKey(key); suggestion != "" {
				attrs = append(attrs, "suggestion", suggestion)
			}
			c.log().Warn("cfggo: unrecognized configuration key (no matching struct field)", attrs...)
		}
		if c.strictKeys {
			return c.WrapError(wrapKind(ErrUnknownKey, fmt.Errorf("%s", c.formatUnknownKeys(unrecognized))), ErrCodeNotFound,
				"unrecognized configuration keys")
		}
	}
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
func (c *Structure) SetErrorWrapper(wrapper cfgerror.Wrapper) {
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

// ensureInit lazily initialises the configuration via InitSelf when an accessor
// (Get, Set, Explain, Diagnose, ...) is called before Init.
//
// IMPORTANT: this is a convenience safety net, not the intended path. Lazy
// initialisation runs with NO options (no config source, no validators, no
// custom logger) and the resulting error can only be logged, because the
// calling method has no way to return it. A program that relies on lazy init
// therefore silently runs on struct/tag defaults only. Always call Init (or
// InitSelf) explicitly at startup so configuration sources are loaded and load
// errors are surfaced
func (c *Structure) ensureInit() {
	if c.initialized.Load() {
		return
	}
	if c.initializing.Load() {
		return
	}
	c.initMutex.Lock()
	defer c.initMutex.Unlock()
	if c.initialized.Load() {
		return
	}
	if c.parent != nil {
		c.initialized.Store(true)
		return
	}
	// Surface the fallback exactly once so a forgotten Init shows up in the
	// logs instead of silently running on struct/tag defaults only
	c.lazyInitWarn.Do(func() {
		c.log().Warn("cfggo: configuration used before Init; falling back to lazy " +
			"initialisation with no options (no config source, validators, or custom logger). " +
			"Call Init/InitSelf explicitly at startup to load sources and surface load errors")
	})
	if err := c.initLocked(c); err != nil {
		c.log().Error("cfggo: lazy initialisation failed: " + err.Error())
		return
	}
	c.initialized.Store(true)
}

// ReloadConfig reloads the configuration from all sources.
func (c *Structure) ReloadConfig() error {
	c.ensureInit()
	c.log().Info("cfggo: reloading configuration")
	return c.Reload()
}

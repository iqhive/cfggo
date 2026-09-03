package cfggo

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/internal/env"
	iflags "github.com/iqhive/cfggo/internal/flags"
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
	// loadedData holds, per key, the value most recently supplied by the
	// configuration source (file, HTTP, bytes) so Save can write it back for a
	// key whose live value is a runtime override from the environment or a
	// command-line flag. Guarded by configMutex
	loadedData  map[string]interface{}
	autoSave    bool
	autoSaveCtx context.Context
	// autoSaveStop ends the auto-save goroutine of a reverted Init so a retried
	// Group.Init does not leave the earlier attempt's goroutine behind
	autoSaveStop chan struct{}

	flagSet *flag.FlagSet
	// externalFlagSet is true when the caller supplied the FlagSet (e.g. via
	// WithFlagSet / WithStandardFlags). In that mode cfggo registers its flags
	// on the supplied set but does NOT parse it: the host owns the single
	// canonical Parse() call, commonly flag.Parse()
	externalFlagSet bool
	// ignoreUnknownVars, when true, makes cfggo ignore command-line flags it
	// doesn't define instead of treating them as an error. Enabled via
	// WithIgnoreUnknownVars. The default (false) makes Init return an error
	// for an unknown flag
	ignoreUnknownVars bool
	// noFlags, when true (via WithoutFlags), skips command-line flag
	// registration and parsing entirely. Programs that configure cfggo purely
	// from files/env/defaults avoid building a flag.FlagSet and a flag.Value
	// per key. It is mutually exclusive with WithFlagSet/WithStandardFlags
	noFlags bool

	// flagNamePrefix is prepended to the flag name (not the configuration key)
	// when registering flags, so several Structures can share one flag.FlagSet
	// without colliding (eg "auth.port"). It is set only by Group; it is empty
	// for every standalone configuration and then costs nothing.
	flagNamePrefix string

	// configPathFlag is the bootstrap flag name supplied to
	// WithFileConfigParamName. It is registered together with the other flags,
	// on whichever flag set is final, so a later WithFlagSet cannot discard it
	configPathFlag string

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

	// reloadMutex serialises Reload against programmatic Set calls. Reload is a
	// multi-phase operation (snapshot, reset, reload, restore, validate) that
	// releases configMutex between phases; without this lock a Set landing
	// mid-reload could be silently lost on a rollback or overwritten by the
	// pre-reload snapshot restore. Set takes the read side (concurrent Sets
	// don't block each other); Reload takes the write side and releases it
	// before invoking OnChange callbacks so a callback may safely call Set
	reloadMutex sync.RWMutex

	// provenance records, per key, where the current value came from. Guarded
	// by configMutex.
	provenance map[string]Source

	// provenanceTrail records, per key, the ordered chain of sources that have
	// contributed to the value (eg default -> file -> env). It is populated
	// lazily: a key that is only ever set by a single source allocates no trail
	// entry, so the common case stays allocation-free. Guarded by configMutex
	provenanceTrail map[string][]Source

	// extraKeys holds the keys registered with NewFlag (key -> help text).
	// They are not backed by a struct field but take part in every layer and
	// are not reported as unrecognized. Guarded by configMutex
	extraKeys map[string]string

	// ignoredKeys holds configuration keys that should be exempt from the
	// "unrecognized key" diagnostics (eg command-line-only flags). Populated via
	// WithIgnoreKeys. It replaces the previous process-global IgnoreFlags so the
	// exemptions are scoped to this instance
	ignoredKeys map[string]bool

	// lazyInitWarn ensures the "used before Init" warning emitted by
	// ensureInit is logged at most once per instance
	lazyInitWarn sync.Once
	initMutex    sync.Mutex
	initializing atomic.Bool
	initialized  atomic.Bool

	// initSnapshot is the state captured before the last successful Init. It
	// lets Group revert a member when a later member fails, so the whole group
	// can be initialised again
	initSnapshot *initStateSnapshot

	// metaMutex guards logger and errorWrapper, which SetLogger and
	// SetErrorWrapper may replace while other goroutines are logging
	metaMutex sync.RWMutex

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
	if c == nil {
		return GlobalErrorWrapper()(nil, ErrCodeInvalidArgument,
			"Init: the embedded *cfggo.Structure is nil; embed cfggo.Structure by value, allocate it, or initialise through cfggo.Init")
	}
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
	c.initSnapshot = &snapshot
	c.initialized.Store(true)
	return nil
}

// revertInit returns a successfully initialised Structure to its pre-Init
// state so Init can be called again. Group uses it when a later member fails,
// so a failed Group.Init can be retried. Accessor closures already installed
// in the parent struct keep working and read the restored default values
func (c *Structure) revertInit() {
	c.initMutex.Lock()
	defer c.initMutex.Unlock()
	if !c.initialized.Load() || c.initSnapshot == nil {
		return
	}
	c.restoreInitState(*c.initSnapshot)
	c.initSnapshot = nil
	if c.autoSaveStop != nil {
		close(c.autoSaveStop)
		c.autoSaveStop = nil
	}
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
	loadedData        map[string]interface{}
	autoSave          bool
	autoSaveCtx       context.Context
	flagSet           *flag.FlagSet
	externalFlagSet   bool
	ignoreUnknownVars bool
	noFlags           bool
	flagNamePrefix    string
	configPathFlag    string
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
	extraKeys         map[string]string
	validationMap     map[string]validcfg.Validator
}

func (c *Structure) snapshotInitState() initStateSnapshot {
	c.configMutex.RLock()
	configData := cloneInterfaceMap(c.configData)
	defaultData := cloneInterfaceMap(c.defaultData)
	loadedData := cloneInterfaceMap(c.loadedData)
	provenance := cloneSourceMap(c.provenance)
	provenanceTrail := cloneSourceTrailMap(c.provenanceTrail)
	extraKeys := cloneStringMap(c.extraKeys)
	c.configMutex.RUnlock()

	c.validationMutex.RLock()
	validationMap := cloneValidatorMap(c.validationMap)
	c.validationMutex.RUnlock()

	c.metaMutex.RLock()
	logger, errorWrapper := c.logger, c.errorWrapper
	c.metaMutex.RUnlock()

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
		loadedData:        loadedData,
		autoSave:          c.autoSave,
		autoSaveCtx:       c.autoSaveCtx,
		flagSet:           c.flagSet,
		externalFlagSet:   c.externalFlagSet,
		ignoreUnknownVars: c.ignoreUnknownVars,
		noFlags:           c.noFlags,
		flagNamePrefix:    c.flagNamePrefix,
		configPathFlag:    c.configPathFlag,
		lenient:           c.lenient,
		strictKeys:        c.strictKeys,
		snakeCaseNames:    c.snakeCaseFieldNames,
		snakeCaseNamesSet: c.snakeCaseFieldNamesSet,
		logger:            logger,
		errorWrapper:      errorWrapper,
		plan:              c.plan,
		provenance:        provenance,
		provenanceTrail:   provenanceTrail,
		ignoredKeys:       cloneBoolMap(c.ignoredKeys),
		extraKeys:         extraKeys,
		validationMap:     validationMap,
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
	c.flagSet = snapshot.flagSet
	c.externalFlagSet = snapshot.externalFlagSet
	c.ignoreUnknownVars = snapshot.ignoreUnknownVars
	c.noFlags = snapshot.noFlags
	c.flagNamePrefix = snapshot.flagNamePrefix
	c.configPathFlag = snapshot.configPathFlag
	c.lenient = snapshot.lenient
	c.strictKeys = snapshot.strictKeys
	c.snakeCaseFieldNames = snapshot.snakeCaseNames
	c.snakeCaseFieldNamesSet = snapshot.snakeCaseNamesSet
	c.metaMutex.Lock()
	c.logger = snapshot.logger
	c.errorWrapper = snapshot.errorWrapper
	c.metaMutex.Unlock()
	c.plan = snapshot.plan
	c.parent = snapshot.parent
	c.initialized.Store(false)

	c.configMutex.Lock()
	if !planBuilt {
		c.changed = snapshot.changed
		c.changeVersion = snapshot.changeVersion
		c.configData = snapshot.configData
		c.defaultData = snapshot.defaultData
		c.loadedData = snapshot.loadedData
		c.provenance = snapshot.provenance
		c.provenanceTrail = snapshot.provenanceTrail
	} else if c.defaultData != nil {
		// When plan application succeeded (wiring accessors and capturing defaults)
		// but a subsequent phase failed (e.g. invalid config file or env var),
		// reset configData and provenance to defaults rather than leaving
		// partially-loaded/poisoned runtime state in place.
		c.configData = make(map[string]interface{}, len(c.defaultData))
		c.provenance = make(map[string]Source, len(c.defaultData))
		c.provenanceTrail = nil
		c.loadedData = nil
		for k, v := range c.defaultData {
			c.configData[k] = cloneMutableInterface(v)
			c.provenance[k] = SourceDefault
		}
		c.changed = snapshot.changed
		c.changeVersion = snapshot.changeVersion
	} else {
		c.changed = snapshot.changed
		c.changeVersion = snapshot.changeVersion
		c.configData = snapshot.configData
		c.defaultData = snapshot.defaultData
		c.loadedData = snapshot.loadedData
		c.provenance = snapshot.provenance
		c.provenanceTrail = snapshot.provenanceTrail
	}
	c.ignoredKeys = snapshot.ignoredKeys
	c.extraKeys = snapshot.extraKeys
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

	c.metaMutex.Lock()
	if c.logger == nil {
		c.logger = GlobalLogger()
	}
	if c.errorWrapper == nil {
		c.errorWrapper = GlobalErrorWrapper()
	}
	c.metaMutex.Unlock()

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
	c.SetLogger(bindConfigName(c.log(), c.name))

	parentType := reflect.TypeOf(c.parent)
	for parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}
	plan, err := planForType(parentType, c.useSnakeCaseFieldNames())
	if err != nil {
		return c.WrapError(err, ErrCodeInvalidArgument, "Init: unsupported configuration struct")
	}
	c.plan = plan
	for _, s := range c.plan.suspects {
		if s.Reason != "" {
			c.log().Warn("cfggo: "+s.Reason+"; the field will be ignored", "key", s.Key, "type", s.Type)
			continue
		}
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
	// private flag set entirely (no flagSet allocation, no per-key flag.Value)
	// for programs that configure purely from files, env, and defaults.
	if c.externalFlagSet {
		c.createFlags()
	} else if !c.noFlags {
		// The private flag set uses flag.ContinueOnError so every flag problem
		// (unknown flag, bad value, --help) is returned from Init as an error
		// rather than terminating the process. Use WithIgnoreUnknownVars to
		// tolerate flags cfggo doesn't define
		c.ensureFlagSet()
		c.createFlags()
		if err := c.parseFlags(); err != nil {
			return err
		}
	}

	if err := c.checkUnrecognizedKeys(); err != nil {
		return err
	}

	if err := c.validateAtInit(); err != nil {
		if !c.lenient {
			c.log().Error("cfggo: configuration validation failed", "err", err)
			return err
		}
		c.log().Warn("cfggo: configuration validation failed", "err", err)
	}

	c.startAutoSave()
	return nil
}

// validateAtInit runs the registered validators at the end of Init.
//
// When the host owns the flag set (WithFlagSet / WithStandardFlags) and has not
// parsed it yet, values supplied on the command line are not visible to cfggo
// until the host calls Parse. A failure for a key whose flag is present in
// os.Args is therefore deferred instead of reported: the flag's own Set
// validates the incoming value during Parse, and Validate() should be called
// afterwards to confirm the final state. Keys with no pending flag are
// validated as usual, so a bad file or environment value still fails Init
func (c *Structure) validateAtInit() error {
	err := c.validate()
	if err == nil || !c.externalFlagSet || c.flagSet == nil || c.flagSet.Parsed() {
		return err
	}
	var errs validcfg.ValidationErrors
	if !errors.As(err, &errs) {
		return err
	}
	args := iflags.FilterTestFlags(os.Args[1:])
	var remaining validcfg.ValidationErrors
	var deferred []string
	for _, ve := range errs {
		if flagPresentInArgs(args, c.flagNamePrefix+ve.Key) {
			deferred = append(deferred, ve.Key)
			continue
		}
		remaining = append(remaining, ve)
	}
	if len(deferred) > 0 {
		sort.Strings(deferred)
		c.log().Info("cfggo: validation deferred for keys supplied on the command line; "+
			"call Validate() after the host parses its flag set", "keys", deferred)
	}
	if len(remaining) == 0 {
		return nil
	}
	return remaining
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
	c.metaMutex.RLock()
	wrapper := c.errorWrapper
	c.metaMutex.RUnlock()
	if wrapper == nil {
		wrapper = GlobalErrorWrapper()
	}
	return wrapper(err, errorcode, msg, args...)
}

// SetLogger sets the instance-specific logger. It is safe to call while other
// goroutines are using the configuration
func (c *Structure) SetLogger(logger cfglogger.Logger) {
	c.metaMutex.Lock()
	c.logger = logger
	c.metaMutex.Unlock()
}

// SetErrorWrapper sets the instance-specific error wrapper. It is safe to call
// while other goroutines are using the configuration
func (c *Structure) SetErrorWrapper(wrapper cfgerror.Wrapper) {
	c.metaMutex.Lock()
	c.errorWrapper = wrapper
	c.metaMutex.Unlock()
}

// GetLogger returns the instance's logger, falling back to the global logger
func (c *Structure) GetLogger() cfglogger.Logger {
	return c.log()
}

// declaredType returns the accessor return type declared for key by the parent
// struct (the T of func() T), or nil when key is not backed by an accessor. A
// key registered with NewFlag has no declared type and yields interface{}, so
// the env and flag layers infer a value for it. It gives those layers a target
// type for a key whose current value is an untyped nil, such as a func()
// interface{} field with no default
func (c *Structure) declaredType(key string) reflect.Type {
	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	return c.declaredTypeLocked(key)
}

// declaredTypeLocked is declaredType for callers that already hold configMutex
func (c *Structure) declaredTypeLocked(key string) reflect.Type {
	if c.plan != nil {
		if leaf, ok := c.plan.byKey[key]; ok && leaf.info.IsAccessor {
			return leaf.info.Type
		}
	}
	if _, ok := c.extraKeys[key]; ok {
		return emptyInterfaceType
	}
	return nil
}

// structure returns the embedded Structure. It lets Group treat any config
// struct that embeds Structure uniformly without reflection.
func (c *Structure) structure() *Structure { return c }

// InitSelf is a convenience variant of Init for when the struct initialises
// itself (i.e. parent == c).
func (c *Structure) InitSelf(options ...Option) error {
	return c.Init(c, options...)
}

// ensureInit is called by the public accessors (Get, Set, Explain, Diagnose,
// ...) so a Structure used before Init behaves predictably: it warns once and
// the method then operates on the empty, uninitialised state (Get reports the
// key as absent, dumps are empty, Set reports an unknown key).
//
// It deliberately initialises nothing. The embedded Structure cannot locate
// the struct that embeds it, so an implicit initialisation could only ever
// produce an empty configuration, and marking the instance initialised would
// then make the application's real Init call fail with ErrAlreadyInitialized.
// Always call Init (or InitSelf) explicitly at startup
func (c *Structure) ensureInit() {
	if c.initialized.Load() || c.initializing.Load() {
		return
	}
	c.lazyInitWarn.Do(func() {
		c.log().Warn("cfggo: configuration used before Init; lookups return zero values and " +
			"accessors are not wired until Init/InitSelf is called explicitly at startup")
	})
}

// ReloadConfig reloads the configuration from all sources.
func (c *Structure) ReloadConfig() error {
	c.ensureInit()
	c.log().Info("cfggo: reloading configuration")
	return c.Reload()
}

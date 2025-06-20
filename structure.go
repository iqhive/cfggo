package cfggo

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"unsafe"

	"sync"
)

var configMutex sync.RWMutex

type Structure struct {
	name               string                 // Name given to this configuration (useful when loading multiple configs)
	configHandler      ConfigHandler          // Configuration handler (optional)
	skipEnv            bool                   // Skip Environment variables
	createdFile        bool                   // Did we create the config file
	changed            bool                   // Has the config changed (used to trigger save on exit)
	defaultsAlreadySet bool                   // Are the defaults already set
	parent             interface{}            // This is a pointer to the parent struct
	configData         map[string]interface{} // Where the configuration data is stored
	autoSave           bool                   // Whether to automatically save on exit

	// Field to store a dedicated FlagSet instead of using flag.CommandLine
	FlagSet *flag.FlagSet

	// Map to store callbacks for boolean flags
	boolCallbacks map[string]func(bool)
}

// DefaultValue returns a function that returns the type of the input parameter X
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

func (c *Structure) Init(parent interface{}, options ...Option) {

	if c.FlagSet == nil {
		c.FlagSet = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		c.FlagSet.Usage = func() {
			fmt.Fprintf(c.FlagSet.Output(), "Usage of %s:\n", os.Args[0])
			c.FlagSet.PrintDefaults()
		}
	}

	// Ensure parent is a pointer
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

	// Apply options
	for _, option := range options {
		err := option(c)
		if err != nil {
			Logger.Errorf("Structure: Init() option returned error: %v", err)
			os.Exit(1)
		}
	}

	if c.name == "" {
		c.name = reflect.TypeOf(c.parent).Elem().Name() // Set c.name as the name of the parent struct
	}

	// Logger.Infof("SetupConfigData %s", c.name)
	c.setupConfigData()

	// Logger.Infof("setDefaultsFromTags %s", c.name)
	c.setDefaultsFromTags()

	// Logger.Infof("ReplaceConfigFuncs %s", c.name)
	c.replaceConfigFuncs()

	// LoadConfig
	if c.configHandler != nil {
		if err := c.loadConfig(false); err != nil {
			Logger.Errorf("LoadConfig: %v", err)
			if c.configHandler != nil && !c.configHandler.IsDefault() {
				os.Exit(1)
			}
		}
	}

	// Logger.Infof("loadFromEnv %s", c.name)
	c.loadFromEnv()

	// Logger.Infof("CreateFlags %s", c.name)
	c.createFlags()

	// Logger.Infof("parseFlags %s", c.name)
	c.parseFlags()

	// Logger.Infof("Validate %s", c.name)
	// Validate configuration after loading from all sources
	if err := c.Validate(); err != nil {
		Logger.Warnf("Configuration validation failed: %v", err)
	}

	// Logger.Info("Done Init")
}

func (c *Structure) InitSelf(options ...Option) {
	// Logger.Infof("InitSelf %s", c.name)
	c.Init(c, options...)
}

// InitMyParent automatically determines the parent struct that contains this Structure
// and calls Init() with that parent.
func (c *Structure) InitMyParent(options ...Option) {
	// Get a pointer to this Structure instance
	structPtr := unsafe.Pointer(reflect.ValueOf(c).Pointer())

	// Find the parent struct by checking memory alignment
	parentValue := reflect.ValueOf(c).Elem().Field(0)
	parentType := parentValue.Type().Elem()

	// Iterate through all fields of the parent type to find the Structure field
	for i := 0; i < parentType.NumField(); i++ {
		field := parentType.Field(i)
		if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
			// Found the Structure field, calculate offset to get parent pointer
			// Keep original pointer alive during arithmetic (go vet requirement)
			offset := field.Offset
			parentPtr := unsafe.Pointer(uintptr(structPtr) - offset)
			parent := reflect.NewAt(parentType, parentPtr).Interface()
			// Call the original Init method with the discovered parent
			c.Init(parent, options...)
			return
		}
	}

	// If we reach here, we couldn't find the parent
	Logger.Error("InitNew(): Could not determine parent struct automatically")
	os.Exit(1)
}

func (c *Structure) setupConfigData() {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	v := reflect.ValueOf(c.parent)

	// Ensure we're working with the struct value, not a pointer
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		Logger.Warnf("SetupConfigData: expected struct, got %v", v.Kind())
		return
	}

	t := v.Type()

	var processStruct func(reflect.Value, reflect.Type, string)
	processStruct = func(v reflect.Value, t reflect.Type, prefix string) {
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
				continue
			}
			fieldValue := v.Field(i)

			configVarName := c.getConfigNameFromField(field)
			if configVarName == "" || configVarName == "-" {
				// Skip fields marked with "-" or empty tag
				continue
			}

			fullKey := configVarName
			if prefix != "" {
				fullKey = prefix + "." + configVarName
			}

			if fieldValue.Kind() == reflect.Struct {
				// Process nested structs recursively
				processStruct(fieldValue, field.Type, fullKey)
			} else if fieldValue.Kind() == reflect.Func && fieldValue.IsNil() {
				// Set the default value in the map, to the reflect.Zero of the type returned from the config function
				c.set(fullKey, reflect.Zero(fieldValue.Type().Out(0)).Interface())
			} else if fieldValue.Kind() == reflect.Func {
				// Set the default value in the map, to the value (and type) returned from the config function
				c.set(fullKey, fieldValue.Call(nil)[0].Interface())
			}
		}
	}

	processStruct(v, t, "")
}

// Set sets a configuration value and then updates the config struct as well
func (c *Structure) Set(key string, value interface{}) error {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.Lock()
	defer configMutex.Unlock()
	c.changed = true
	return c.set(key, value)
}

// set is a private function that sets a configuration value without locking
func (c *Structure) set(key string, value interface{}) error {
	if existing, exists := c.configData[key]; exists {
		existingType := reflect.TypeOf(existing)

		// Use the helper function for type conversion
		convertedValue, err := ConvertValue(value, existingType)
		if err != nil {
			return err
		}

		c.configData[key] = convertedValue
		return nil
	}

	c.configData[key] = value
	return nil
}

// Helper function to check if a type is numeric
func isNumericType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// Get gets a configuration value and whether it exists from the configData
func (c *Structure) Get(key string) (interface{}, bool) {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	value, exists := c.configData[key]
	return value, exists
}

func (c *Structure) createFlags() {
	for key, value := range c.configData {
		// Fetch the description from the struct tag
		configDescription := c.GetHelpTag(key)
		c.NewFlag(key, value, configDescription)
	}
}

func (c *Structure) replaceConfigFuncs() {
	configMutex.RLock()
	defer configMutex.RUnlock()

	v := reflect.ValueOf(c.parent)

	// Keep dereferencing until we get to a non-pointer value
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			Logger.Warn("ReplaceConfigFuncs: received nil pointer")
			return
		}
		v = v.Elem()
	}

	// Ensure we're working with a struct
	if v.Kind() != reflect.Struct {
		Logger.Warnf("ReplaceConfigFuncs: expected struct or pointer to struct, got %v", v.Kind())
		return
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if fieldValue.Kind() == reflect.Func {
			configVarName := c.getConfigNameFromField(field)

			// Skip fields marked with "-"
			if configVarName == "-" {
				continue
			}

			if configVarName != "" && configVarName != "-" {
				if _, exists := c.configData[configVarName]; !exists {
					Logger.Errorf("Missing configData value for key %s", configVarName)
					continue
				}

				// Create a closure that captures the config variable name correctly
				// This is critical to avoid all functions returning the same value
				func(capturedConfigVarName string) {
					fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(args []reflect.Value) (results []reflect.Value) {
						// Get a fresh read lock for each function call to ensure thread safety
						configMutex.RLock()
						defer configMutex.RUnlock()
						return []reflect.Value{reflect.ValueOf(c.configData[capturedConfigVarName])}
					}))
				}(configVarName)
			}
		}
	}
}

// configNameCache caches the results of getConfigNameFromField
var configNameCache = make(map[string]string)
var configNameCacheMutex sync.RWMutex

// Initialize the cache safely
func init() {
	configNameCache = make(map[string]string)
}

// getFieldKey creates a unique string key for a StructField
func getFieldKey(field reflect.StructField) string {
	return field.PkgPath + "." + field.Name + ":" + string(field.Tag)
}

func (c *Structure) getConfigNameFromField(field reflect.StructField) string {
	fieldKey := getFieldKey(field)

	configNameCacheMutex.RLock()
	if name, exists := configNameCache[fieldKey]; exists {
		configNameCacheMutex.RUnlock()
		return name
	}
	configNameCacheMutex.RUnlock()

	// First check for "cfggo" tag (default)
	configVarName := field.Tag.Get("cfggo")
	if configVarName == "" {
		configVarName = field.Tag.Get("cfg") // (backwards compatibility)
	}
	if configVarName == "" {
		configVarName = field.Tag.Get("config") // (backwards compatibility)
	}
	if configVarName == "" {
		configVarName = field.Tag.Get("json") // (fallback)
	}
	if configVarName == "" {
		configVarName = field.Name // (fallback)
	}

	// If the tag contains a comma, take only the part before the comma
	// (to handle json tags like `json:"name,omitempty"`)
	if idx := strings.Index(configVarName, ","); idx != -1 {
		configVarName = configVarName[:idx]
	}

	configNameCacheMutex.Lock()
	configNameCache[fieldKey] = configVarName
	configNameCacheMutex.Unlock()

	return configVarName
}

func (c *Structure) getAllKeys() []string {
	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}
	return keys
}

func (c *Structure) setDefaultsFromTags() {
	if c.defaultsAlreadySet {
		return
	}
	c.defaultsAlreadySet = true
	v := reflect.ValueOf(c.parent)

	// Ensure we're working with the struct value, not a pointer
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		Logger.Warnf("SetDefaults: expected struct, got %v", v.Kind())
		return
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		configVarName := c.getConfigNameFromField(field)
		if configVarName == "" || configVarName == "-" {
			continue
		}

		if fieldValue.Kind() != reflect.Func || !fieldValue.IsNil() {
			continue
		}

		// Attempt to read the "default" tag
		if defaultStr, ok := field.Tag.Lookup("default"); ok && defaultStr != "" {
			// Parse defaultStr into the proper type using a dynamicVar
			dv := &dynamicVar{
				config: c,
				name:   configVarName,
				want:   fieldValue.Type().Out(0), // the return type of the function
			}
			// If parsing fails, log a warning but continue
			if err := dv.Set(defaultStr); err != nil {
				Logger.Warnf("SetDefaults: could not parse default value for field %s: %v", field.Name, err)
			}
		}
	}
}

// ReloadConfig reloads the configuration from sources
func (c *Structure) ReloadConfig() error {
	if c.parent == nil {
		c.InitSelf()
	}
	Logger.Infof("ReloadConfig %s", c.name)

	return c.Reload()
}

package cfggo

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"sync"
)

var configMutex sync.RWMutex

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		// fmt.Println("Configuration parameters:")
		// for name := range flag.CommandLine.NFlag() {
		// 	fmt.Printf("  %s\n", name)
		// }
	}
}

type Structure struct {
	name               string                 // Name given to this configuration (useful when loading multiple configs)
	configHandler      configHandler          // Configuration handler (optional)
	skipEnv            bool                   // Skip Environment variables
	createdFile        bool                   // Did we create the config file
	changed            bool                   // Has the config changed (used to trigger save on exit)
	defaultsAlreadySet bool                   // Are the defaults already set
	parent             interface{}            // This is a pointer to the parent struct
	configData         map[string]interface{} // Where the configuration data is stored
	autoSave           bool                   // Whether to automatically save on exit

	// New field to store a dedicated FlagSet instead of using flag.CommandLine
	FlagSet *flag.FlagSet
}

// DefaultValue returns a function that returns the type of the input parameter X
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

func (c *Structure) Init(parent interface{}, options ...Option) {

	if c.FlagSet == nil {
		c.FlagSet = flag.NewFlagSet("cfggo", flag.ExitOnError)
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
			Logger.Error("Structure: Init() option returned error: %v", err)
			os.Exit(1)
		}
	}

	if c.name == "" {
		c.name = reflect.TypeOf(c.parent).Elem().Name() // Set c.name as the name of the parent struct
	}

	// Logger.Info("SetupConfigData %s", name)
	c.setupConfigData()

	// // Logger.Info("setDefaultsFromTags %s", name)
	c.setDefaultsFromTags()

	// Logger.Info("ReplaceConfigFuncs %s", name)
	c.replaceConfigFuncs()

	// LoadConfig
	if c.configHandler != nil {
		c.loadConfig()
	}

	// Logger.Info("loadFromEnv %s", name)
	c.loadFromEnv()

	// Logger.Info("CreateFlags %s", name)
	c.createFlags()

	c.parseFlags()

	// Logger.Info("Done Init")
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
		Logger.Warn("SetupConfigData: expected struct, got %v", v.Kind())
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
	configMutex.Lock()
	defer configMutex.Unlock()
	c.changed = true
	return c.set(key, value)
}

// set is a private function that sets a configuration value without locking
func (c *Structure) set(key string, value interface{}) error {
	if existing, exists := c.configData[key]; exists {
		existingType := reflect.TypeOf(existing)
		valueType := reflect.TypeOf(value)

		// If types don't match but are convertible
		if valueType != existingType {
			valueValue := reflect.ValueOf(value)

			// Special handling for time.Duration
			if existingType == reflect.TypeOf(time.Duration(0)) {
				switch valueType.Kind() {
				case reflect.String:
					duration, err := time.ParseDuration(valueValue.String())
					if err == nil {
						c.configData[key] = duration
						return nil
					}
				case reflect.Int, reflect.Int64:
					// Only convert to Duration if we're sure this is meant to be a Duration
					// and not a regular int64
					c.configData[key] = time.Duration(valueValue.Int())
					return nil
				case reflect.Float64:
					c.configData[key] = time.Duration(int64(valueValue.Float()))
					return nil
				}
			} else if existingType == reflect.TypeOf(int64(0)) {
				// Ensure int64 values don't get mistakenly converted to Duration
				switch valueType.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					c.configData[key] = valueValue.Int()
					return nil
				case reflect.Float64:
					c.configData[key] = int64(valueValue.Float())
					return nil
				case reflect.String:
					intVal, err := strconv.ParseInt(valueValue.String(), 10, 64)
					if err == nil {
						c.configData[key] = intVal
						return nil
					}
				}
			}

			// Handle numeric type conversions
			if isNumericType(existingType) && isNumericType(valueType) {
				switch existingType.Kind() {
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					var intVal int64
					switch valueType.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						intVal = valueValue.Int()
					case reflect.Float32, reflect.Float64:
						intVal = int64(valueValue.Float())
					case reflect.String:
						var err error
						intVal, err = strconv.ParseInt(valueValue.String(), 10, 64)
						if err != nil {
							return ErrorWrapper(err, 400, "Cannot convert string to int: %v", err)
						}
					}
					newValue := reflect.New(existingType).Elem()
					newValue.SetInt(intVal)
					c.configData[key] = newValue.Interface()
					return nil

				case reflect.Float32, reflect.Float64:
					var floatVal float64
					switch valueType.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						floatVal = float64(valueValue.Int())
					case reflect.Float32, reflect.Float64:
						floatVal = valueValue.Float()
					case reflect.String:
						var err error
						floatVal, err = strconv.ParseFloat(valueValue.String(), 64)
						if err != nil {
							return ErrorWrapper(err, 400, "Cannot convert string to float: %v", err)
						}
					}
					newValue := reflect.New(existingType).Elem()
					newValue.SetFloat(floatVal)
					c.configData[key] = newValue.Interface()
					return nil
				}
			}

			// Handle boolean conversions
			if existingType.Kind() == reflect.Bool && valueType.Kind() == reflect.String {
				strVal := strings.ToLower(valueValue.String())
				if strVal == "true" || strVal == "t" || strVal == "yes" || strVal == "y" || strVal == "1" {
					c.configData[key] = true
					return nil
				} else if strVal == "false" || strVal == "f" || strVal == "no" || strVal == "n" || strVal == "0" {
					c.configData[key] = false
					return nil
				}
			}

			// Try standard conversion if types are convertible
			if valueType.ConvertibleTo(existingType) {
				value = valueValue.Convert(existingType).Interface()
			} else {
				return ErrorWrapper(nil, 400, "Type mismatch for key %s: %T != %T", key, value, existing)
			}
		}
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
		Logger.Warn("ReplaceConfigFuncs: expected struct or pointer to struct, got %v", v.Kind())
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
					Logger.Error("Missing configData value for key %s", configVarName)
					continue
				}

				fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(args []reflect.Value) (results []reflect.Value) {
					return []reflect.Value{reflect.ValueOf(c.configData[configVarName])}
				}))
			}
		}
	}
}

// create struct create a new struct based on the config data
func (c *Structure) createStruct() interface{} {
	ptype := reflect.TypeOf(c.parent).Elem() // always a pointer.
	fields := make([]reflect.StructField, 0, ptype.NumField())

	for i := 0; i < ptype.NumField(); i++ {
		field := ptype.Field(i)
		if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
			continue
		}

		configKey := c.getConfigNameFromField(field)
		newField := reflect.StructField{
			Name: field.Name,
			Type: field.Type,
			Tag:  reflect.StructTag(`json:"` + configKey + `"`),
		}

		// Handle nested structs by creating new types with adjusted fields
		if newField.Type.Kind() == reflect.Struct {
			newField.Type = c.createNestedStructType(newField.Type)
		}

		if newField.Type.Kind() == reflect.Func && newField.Type.NumIn() == 0 && newField.Type.NumOut() == 1 {
			newField.Type = newField.Type.Out(0)
		}

		fields = append(fields, newField)
	}

	resp := reflect.New(reflect.StructOf(fields)).Interface()

	// Add values to the struct
	rvalue := reflect.ValueOf(resp).Elem()
	rtype := rvalue.Type()
	for i := range rvalue.NumField() {
		field := rtype.Field(i)
		configKey := c.getConfigNameFromField(field)
		if configKey == "" || configKey == "-" {
			continue
		}
		// fmt.Printf("setting default struct field value %s to %v\n", configKey, c.configData[configKey])
		rvalue.Field(i).Set(reflect.ValueOf(c.configData[configKey]))
	}
	return resp
}

func (c *Structure) createNestedStructType(t reflect.Type) reflect.Type {
	fields := make([]reflect.StructField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		configKey := c.getConfigNameFromField(field)
		newField := reflect.StructField{
			Name: field.Name,
			Type: field.Type,
			Tag:  reflect.StructTag(`json:"` + configKey + `"`),
		}

		// Recursively process nested structs
		if newField.Type.Kind() == reflect.Struct {
			newField.Type = c.createNestedStructType(newField.Type)
		}

		fields = append(fields, newField)
	}
	return reflect.StructOf(fields)
}

func (c *Structure) getConfigNameFromField(field reflect.StructField) string {
	// First check for "cfg" tag
	configVarName := field.Tag.Get("cfg")
	if configVarName == "" {
		// Then check for "json" tag
		configVarName = field.Tag.Get("json")
		if configVarName == "" {
			// Finally use the field name
			configVarName = field.Name
		}
	}

	// If the tag contains a comma, take only the part before the comma
	// (to handle json tags like `json:"name,omitempty"`)
	if idx := strings.Index(configVarName, ","); idx != -1 {
		configVarName = configVarName[:idx]
	}

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
		Logger.Warn("SetDefaults: expected struct, got %v", v.Kind())
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
				Logger.Warn("SetDefaults: could not parse default value for field %s: %v", field.Name, err)
			}
		}
	}
}

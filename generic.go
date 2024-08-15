package cfggo

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
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
	filename           string                 // Config filename, eg: application.json
	createdFile        bool                   // Did we create the config file
	changed            bool                   // Has the config changed (used to trigger save on exit)
	defaultsAlreadySet bool                   // Are the defaults already set
	parent             interface{}            // This is a pointer to the parent struct
	configData         map[string]interface{} // Where the configuration data is stored
}

// DefaultValue returns a function that returns the type of the input parameter X
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

func (c *Structure) Init(parent interface{}, name string, filename string) {

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
	c.SetName(name)
	c.SetFilename(filename)

	// Logger.Info("SetupConfigData %s", name)
	c.setupConfigData()

	// Logger.Info("ReplaceConfigFuncs %s", name)
	c.replaceConfigFuncs()

	// Logger.Info("SetDefaults %s", name)
	c.setDefaults()

	// Logger.Info("loadConfig %s", name)
	c.loadConfig()

	// Logger.Info("loadFromEnv %s", name)
	c.loadFromEnv()

	// Logger.Info("CreateFlags %s", name)
	c.createFlags()

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
	// if v.Kind() != reflect.Struct {
	// 	Logger.Error("SetupConfigData: parent is not a pointer to a struct")
	// 	os.Exit(1)
	// }

	if v.Kind() != reflect.Struct {
		Logger.Warn("SetupConfigData: expected struct, got %v", v.Kind())
		return
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
			continue
		}
		fieldValue := v.Field(i)

		configVarName := c.getConfigNameFromField(field)
		if configVarName == "" || configVarName == "-" {
			continue
		}

		if configVarName == "" {
			Logger.Warn("No config or json tag found for field %s", field.Name)
			configVarName = t.Field(i).Name
		}

		if fieldValue.Kind() == reflect.Func && fieldValue.IsNil() {
			// Set the default value in the map, to the reflect.Zero of the type returned from the config function
			c.set(configVarName, reflect.Zero(fieldValue.Type().Out(0)).Interface())
		} else if fieldValue.Kind() == reflect.Func {
			// Set the default value in the map, to the value (and type) returned from the config function
			c.set(configVarName, fieldValue.Call(nil)[0].Interface())
		}
	}
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
		if reflect.TypeOf(value) != reflect.TypeOf(existing) && reflect.TypeOf(value).ConvertibleTo(reflect.TypeOf(existing)) {
			value = reflect.ValueOf(value).Convert(reflect.TypeOf(existing)).Interface()
		}
		if reflect.TypeOf(value) != reflect.TypeOf(existing) {
			return ErrorWrapper(nil, 400, "Type mismatch for key %s: %T != %T", key, value, existing)
		}
	}
	c.configData[key] = value
	return nil
}

func (c *Structure) setEmptySlice(key string) {
	if existingVal, exists := c.configData[key]; exists {
		c.setValue(key, reflect.MakeSlice(reflect.TypeOf(existingVal), 0, 0).Interface())
	} else {
		Logger.Error("Missing key in configData for %s", key)
		c.configData[key] = nil
	}
}

func (c *Structure) setValue(key string, value interface{}) {
	c.configData[key] = value
}

// Get gets a configuration value and whether it exists from the configData
func (c *Structure) Get(key string) (interface{}, bool) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	value, exists := c.configData[key]
	return value, exists
}

func (c *Structure) SetFilename(filename string) error {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}
	if c.parent == nil {
		Logger.Error("SetFilename should only ever be called after Init()")
		os.Exit(1)
	}

	c.filename = filename

	return nil
}

func (c *Structure) SetName(name string) {
	if c.parent == nil {
		Logger.Error("SetName should only ever be called after Init()")
		os.Exit(1)
	}
	c.name = name
}

func (c *Structure) createFlags() {
	for key, value := range c.configData {
		configDescription := "" // You can set a default description or fetch it from somewhere if needed
		c.NewVar(key, value, configDescription)
	}
}

// var once sync.Once
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
			configVarName := field.Tag.Get("config")
			if configVarName == "" {
				configVarName = field.Tag.Get("json")
			}

			if _, exists := c.configData[configVarName]; !exists {
				Logger.Error("Missing configData value for key %s", configVarName)
				continue
			}

			if configVarName != "" && configVarName != "-" {
				// Logger.Debugf("making Func %s of type %s", configVarName, fieldValue.Type())
				fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(args []reflect.Value) (results []reflect.Value) {
					return []reflect.Value{reflect.ValueOf(c.configData[configVarName])}
				}))
			}
		}
	}
}

func (c *Structure) loadFromEnv() {
	configMutex.Lock()
	defer configMutex.Unlock()

	for key := range c.configData {
		envVar := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		if value, exists := os.LookupEnv(envVar); exists {
			var err error
			if reflect.TypeOf(c.configData[key]) == reflect.TypeOf(time.Time{}) {
				var t time.Time
				t, err = time.Parse(time.RFC3339, value)
				if err == nil {
					// Logger.Warn("loadFromEnv time.Parse %s of type %s", key, reflect.TypeOf(c.configData[key]))
					err = c.set(key, t)
				}
			} else if reflect.TypeOf(c.configData[key]) == reflect.TypeOf(time.Duration(0)) {
				var d time.Duration
				d, err = time.ParseDuration(value)
				if err == nil {
					// Logger.Warn("loadFromEnv time.ParseDuration %s of type %s", key, reflect.TypeOf(c.configData[key]))
					err = c.set(key, d)
				}
			} else {
				// Logger.Warn("loadFromEnv set %s=(%s) of type %s", key, value, reflect.TypeOf(c.configData[key]))
				err = c.set(key, value)
			}
			if err != nil {
				Logger.Info("Error setting config from environment variable %s: %v", envVar, err)
			}
		}
	}
}

// create struct create a new struct based on the config data
func (c *Structure) createStruct() interface{} {
	ptype := reflect.TypeOf(c.parent).Elem() // always a pointer.
	fields := make([]reflect.StructField, 0)
	for i := range ptype.NumField() {
		field := ptype.Field(i)
		if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
			continue
		}
		if field.Type.Kind() == reflect.Func && field.Type.NumIn() == 0 && field.Type.NumOut() == 1 {
			field.Type = field.Type.Out(0) // transform 'func() T' to 'T'
		}
		fields = append(fields, field)
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

func (c *Structure) getConfigNameFromField(field reflect.StructField) string {
	if name, ok := field.Tag.Lookup("config"); ok {
		return name
	}
	if name, ok := field.Tag.Lookup("json"); ok {
		name, _, _ := strings.Cut(name, ",")
		return name
	}
	return field.Name
}

func (c *Structure) loadConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()

	if c.filename == "" {
		return nil
	}
	file, err := os.Open(c.filename)
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}
	defer file.Close()

	tempConfigData := c.createStruct()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(tempConfigData); err != nil {
		return ErrorWrapper(err, 0, "")
	}

	rvalue := reflect.ValueOf(tempConfigData).Elem()
	rtype := rvalue.Type()
	for i := range rvalue.NumField() {
		field := rtype.Field(i)
		configKey := c.getConfigNameFromField(field)
		if configKey == "" || configKey == "-" {
			continue
		}
		err := c.set(configKey, rvalue.Field(i).Interface())
		if err != nil {
			Logger.Warn("loadConfig error setting %s to (%v): %v", configKey, rvalue.Field(i).Interface(), err)
		}
	}

	return nil
}

func (c *Structure) getAllKeys() []string {
	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}
	return keys
}

func (c *Structure) setDefaults() {
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

		if fieldValue.Kind() == reflect.Func && fieldValue.IsNil() {
			configVarName := field.Tag.Get("config")
			if configVarName == "" {
				configVarName = field.Tag.Get("json")
			}

			if configVarName != "" && configVarName != "-" {
				fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(args []reflect.Value) (results []reflect.Value) {
					return []reflect.Value{reflect.Zero(fieldValue.Type().Out(0))}
				}))
			}
		}
	}
}

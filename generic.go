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

type GenericConfig struct {
	configData         map[string]interface{}
	name               string
	filename           string
	createdFile        bool
	changed            bool
	defaultsAlreadySet bool
	parent             interface{}
}

// DefaultValue returns a function that returns the type of the input parameter X
func DefaultValue[T any](x T) func() T {
	return func() T {
		return x
	}
}

func (c *GenericConfig) Init(parent interface{}, name string, filename string) {

	// Ensure parent is a pointer
	v := reflect.ValueOf(parent)
	if v.Kind() != reflect.Ptr {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		parent = ptr.Interface()
		Logger.Warn("GenericConfig: Init() must be called with a parent struct pointer, not a struct")
	}

	if c.parent != nil {
		Logger.Warn("GenericConfig: Init() called more than once")
		return
	}
	c.parent = parent
	c.SetName(name)
	c.SetFilename(filename)

	// Logger.Info("SetupConfigData %s", name)
	c.SetupConfigData()

	// Logger.Info("ReplaceConfigFuncs %s", name)
	c.ReplaceConfigFuncs()

	// Logger.Info("SetDefaults %s", name)
	c.SetDefaults()

	// Logger.Info("loadConfig %s", name)
	c.loadConfig()

	// Logger.Info("loadFromEnv %s", name)
	c.loadFromEnv()

	// Logger.Info("CreateFlags %s", name)
	c.CreateFlags()

	// Logger.Info("loadFlags %s", name)
	// c.loadFlags()

	// Logger.Info("Done Init")
}

func (c *GenericConfig) SetupConfigData() {
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

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.Name == "GenericConfig" {
			continue
		}
		fieldValue := v.Field(i)

		configVarName := field.Tag.Get("config")
		if configVarName == "" {
			configVarName = field.Tag.Get("json")
		}

		if configVarName == "-" {
			continue
		}

		if configVarName == "" {
			Logger.Warn("No config or json tag found for field %s", field.Name)
			configVarName = t.Field(i).Name
		}

		if fieldValue.Kind() == reflect.Func && fieldValue.IsNil() {
			c.setDefaultValue(configVarName, fieldValue.Type().Out(0))
		} else if fieldValue.Kind() == reflect.Func {
			c.setValueFromFunc(configVarName, fieldValue)
		}
	}

}

func (c *GenericConfig) setDefaultValue(configVarName string, kind reflect.Type) {
	switch kind.Kind() {
	case reflect.String:
		c.set(configVarName, "")
	case reflect.Bool:
		c.set(configVarName, false)
	case reflect.Int:
		c.set(configVarName, 0)
	case reflect.Int64:
		if kind == reflect.TypeOf(time.Duration(0)) {
			c.set(configVarName, time.Duration(0))
		} else {
			c.set(configVarName, int64(0))
		}
	case reflect.Float64:
		c.set(configVarName, 0.0)
	case reflect.Float32:
		c.set(configVarName, float32(0.0))
	case reflect.Slice:
		c.set(configVarName, reflect.MakeSlice(kind, 0, 0).Interface())
	case reflect.Map:
		c.set(configVarName, reflect.MakeMap(kind).Interface())
	case reflect.Struct:
		if kind == reflect.TypeOf(time.Time{}) {
			c.set(configVarName, time.Time{})
		}
	}
}

func (c *GenericConfig) setValueFromFunc(configVarName string, fieldValue reflect.Value) {
	// Logger.Warn("setValueFromFunc %s of type %s", configVarName, fieldValue.Type())
	c.set(configVarName, fieldValue.Call(nil)[0].Interface())
}

func (c *GenericConfig) loadFlags() {
	flag.Parse()
}

// Set sets a configuration value and then updates the config struct as well
func (c *GenericConfig) Set(key string, value interface{}) error {
	configMutex.Lock()
	defer configMutex.Unlock()
	c.changed = true
	return c.set(key, value)
}

// set is a private function that sets a configuration value without locking
func (c *GenericConfig) set(key string, value interface{}) error {
	// before := fmt.Sprintf("%T", c.configData[key])
	// if before != "<nil>" {
	// 	Logger.Info("set before type: %s", before)
	// }
	// Logger.Info("Setting %s", key)

	switch val := value.(type) {
	case nil:
		if existingVal, exists := c.configData[key]; exists {
			switch reflect.TypeOf(existingVal).Kind() {
			case reflect.Slice:
				c.setValue(key, reflect.MakeSlice(reflect.TypeOf(existingVal), 0, 0).Interface())
			case reflect.Map:
				// Logger.Debugf("making map %s of type %T", key, existingVal)
				c.setValue(key, reflect.MakeMap(reflect.TypeOf(existingVal)).Interface())
			default:
				return ErrorWrapper(nil, 400, "nil is not a valid type for key %s", key)
				// c.setValue(key, nil)
			}
		} else {
			return ErrorWrapper(nil, 400, "nil set into unknown key %s", key)
			// c.setValue(key, nil)
		}
	case time.Duration:
		c.setValue(key, time.Duration(val))
	case string, bool, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		c.setValue(key, val)
	case float32:
		c.setValue(key, float64(val))
	case time.Time:
		c.setValue(key, val)
	case []int, []string, []bool, []float64, []float32:
		if val == nil {
			c.setValue(key, reflect.MakeSlice(reflect.TypeOf(val), 0, 0).Interface())
		} else {
			c.setValue(key, val)
		}
	case map[string]interface{}:

		if val == nil || len(val) == 0 {
			c.setValue(key, reflect.MakeMap(reflect.TypeOf(val)).Interface())
		} else {
			c.setValue(key, val)
		}

	case map[string]string:
		if val == nil || len(val) == 0 {
			c.setValue(key, reflect.MakeMap(reflect.TypeOf(val)).Interface())
		} else {
			c.setValue(key, val)
		}
	case []interface{}:
		if val == nil {
			c.setValue(key, []interface{}{})
		} else if len(val) > 0 {
			switch val[0].(type) {
			case int:
				c.setValue(key, convertToIntSlice(val))
			case string:
				c.setValue(key, convertToStringSlice(val))
			case bool:
				c.setValue(key, convertToBoolSlice(val))
			case float64:
				c.setValue(key, convertToFloat64Slice(val))
			case float32:
				c.setValue(key, convertToFloat32Slice(val))
			case int64:
				c.setValue(key, convertToInt64Slice(val))
			default:
				return ErrorWrapper(fmt.Errorf("Unsupported slice type for key %s: %T", key, val[0]), 0, "")
			}
		} else {
			c.setEmptySlice(key)
		}
	default:
		return ErrorWrapper(nil, 400, "Unsupported type for key %s: %T", key, value)
	}

	// after := fmt.Sprintf("%T", c.configData[key])
	// if before != "<nil>" && before != after {
	// 	Logger.Warn("set WARNING: before type: %s != after type: %s", before, after)
	// }

	return nil
}

func (c *GenericConfig) setEmptySlice(key string) {
	if existingVal, exists := c.configData[key]; exists {
		c.setValue(key, reflect.MakeSlice(reflect.TypeOf(existingVal), 0, 0).Interface())
	} else {
		Logger.Error("Missing key in configData for %s", key)
		c.configData[key] = nil
	}
}

func (c *GenericConfig) setValue(key string, value interface{}) {
	c.configData[key] = value
}

func convertToIntSlice(val []interface{}) []int {
	intSlice := make([]int, len(val))
	for i, v := range val {
		intSlice[i] = v.(int)
	}
	return intSlice
}

func convertToStringSlice(val []interface{}) []string {
	stringSlice := make([]string, len(val))
	for i, v := range val {
		stringSlice[i] = v.(string)
	}
	return stringSlice
}

func convertToBoolSlice(val []interface{}) []bool {
	boolSlice := make([]bool, len(val))
	for i, v := range val {
		boolSlice[i] = v.(bool)
	}
	return boolSlice
}

func convertToFloat64Slice(val []interface{}) []float64 {
	float64Slice := make([]float64, len(val))
	for i, v := range val {
		float64Slice[i] = v.(float64)
	}
	return float64Slice
}

func convertToFloat32Slice(val []interface{}) []float32 {
	float32Slice := make([]float32, len(val))
	for i, v := range val {
		float32Slice[i] = v.(float32)
	}
	return float32Slice
}

func convertToInt64Slice(val []interface{}) []int64 {
	int64Slice := make([]int64, len(val))
	for i, v := range val {
		int64Slice[i] = v.(int64)
	}
	return int64Slice
}

// Get gets a configuration value and whether it exists from the configData
func (c *GenericConfig) Get(key string) (interface{}, bool) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	value, exists := c.configData[key]
	return value, exists
}

func (c *GenericConfig) SetFilename(filename string) error {
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

func (c *GenericConfig) SetName(name string) {
	if c.parent == nil {
		Logger.Error("SetName should only ever be called after Init()")
		os.Exit(1)
	}
	c.name = name
}

func (c *GenericConfig) CreateFlags() {
	for key, value := range c.configData {
		configDescription := "" // You can set a default description or fetch it from somewhere if needed
		c.NewVar(key, value, configDescription)
	}
}

// var once sync.Once
func (c *GenericConfig) ReplaceConfigFuncs() {
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
					return []reflect.Value{c.convertConfigValue(configVarName, fieldValue.Type().Out(0))}
				}))
			}
		}
	}
}

func (c *GenericConfig) convertConfigValue(key string, kind reflect.Type) reflect.Value {
	value := c.configData[key]
	fmt.Printf("key (%s) = (%T) kind: %s\n", key, value, kind.Kind())
	Logger.Info("key (%s) = (%T) kind: %s", key, value, kind.Kind())
	switch kind.Kind() {
	case reflect.Int:
		return reflect.ValueOf(value.(int))
	case reflect.Int8:
		return reflect.ValueOf(int8(value.(int)))
	case reflect.Int16:
		return reflect.ValueOf(int16(value.(int)))
	case reflect.Int32:
		return reflect.ValueOf(int32(value.(int)))
	case reflect.Int64:
		if kind == reflect.TypeOf(time.Duration(0)) {
			return reflect.ValueOf(value.(time.Duration))
		}
		return reflect.ValueOf(value.(int64))
	case reflect.Uint:
		return reflect.ValueOf(value.(uint))
	case reflect.Uint8:
		return reflect.ValueOf(uint8(value.(uint)))
	case reflect.Uint16:
		return reflect.ValueOf(uint16(value.(uint)))
	case reflect.Uint32:
		return reflect.ValueOf(uint32(value.(uint)))
	case reflect.Uint64:
		return reflect.ValueOf(value.(uint64))
	case reflect.Struct:
		if kind == reflect.TypeOf(time.Time{}) {
			return reflect.ValueOf(value.(time.Time))
		}
	case reflect.Map:
		if kind == reflect.TypeOf(map[string]string{}) {
			return reflect.ValueOf(value.(map[string]string))
		} else if kind == reflect.TypeOf(map[string]interface{}{}) {
			return reflect.ValueOf(value.(map[string]interface{}))
		}
	case reflect.Slice:
		switch kind.Elem().Kind() {
		case reflect.Int:
			return reflect.ValueOf(value.([]int))
		case reflect.Int8:
			return reflect.ValueOf(value.([]int8))
		case reflect.Int16:
			return reflect.ValueOf(value.([]int16))
		case reflect.Int32:
			return reflect.ValueOf(value.([]int32))
		case reflect.Int64:
			return reflect.ValueOf(value.([]int64))
		case reflect.Uint:
			return reflect.ValueOf(value.([]uint))
		case reflect.Uint8:
			return reflect.ValueOf(value.([]uint8))
		case reflect.Uint16:
			return reflect.ValueOf(value.([]uint16))
		case reflect.Uint32:
			return reflect.ValueOf(value.([]uint32))
		case reflect.Uint64:
			return reflect.ValueOf(value.([]uint64))
		case reflect.Float32:
			return reflect.ValueOf(value.([]float32))
		case reflect.Float64:
			return reflect.ValueOf(value.([]float64))
		case reflect.Bool:
			return reflect.ValueOf(value.([]bool))
		case reflect.String:
			return reflect.ValueOf(value.([]string))
		}
	default:
		return reflect.ValueOf(value)
	}
	return reflect.Value{}
}

func (c *GenericConfig) loadFromEnv() {
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
func (c *GenericConfig) loadConfig() error {
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

	tempConfigData := make(map[string]interface{})
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&tempConfigData); err != nil {
		return ErrorWrapper(err, 0, "")
	}

	for key, value := range tempConfigData {
		// Logger.Warn("loadConfig setting %s with type %T", key, value)
		if reflect.TypeOf(value) == nil {
			Logger.Warn("loadConfig is skipping nil value for %s", key)
			continue
		}
		var err error
		if reflect.TypeOf(c.configData[key]) == reflect.TypeOf(time.Time{}) {
			// Logger.Debugf("loadConfig Setting time.Time %s", key)
			var t time.Time
			t, err = time.Parse(time.RFC3339, value.(string))
			if err == nil {
				err = c.set(key, t)
			}
		} else if reflect.TypeOf(c.configData[key]) == reflect.TypeOf(time.Duration(0)) {
			// Logger.Debugf("loadConfig Setting time.Duration %s", key)
			var d time.Duration
			d, err = time.ParseDuration(value.(string))
			if err == nil {
				err = c.set(key, d)
			}
		} else if reflect.TypeOf(c.configData[key]) == reflect.TypeOf(map[string]string{}) {
			// Logger.Debugf("loadConfig Setting map[string]string %s to type %T", key, value)
			// Convert map[string]interface{} to map[string]string
			convertedMap := make(map[string]string)
			for k, v := range value.(map[string]interface{}) {
				convertedMap[k] = v.(string)
			}
			err = c.set(key, convertedMap)
		} else {
			// Logger.Debugf("loadConfig Setting something else %s (%T)", key, value)
			err = c.set(key, value)
		}
		if err != nil {
			Logger.Error("loadConfig ERROR: %v", err)
			continue
		}
	}
	// Logger.Debugf("loadConfig Done")
	return nil
}

func (c *GenericConfig) getAllKeys() []string {
	keys := make([]string, 0, len(c.configData))
	for key := range c.configData {
		keys = append(keys, key)
	}
	return keys
}

func (c *GenericConfig) getString(key string) string {
	if value, ok := c.configData[key].(string); ok {
		return value
	}
	return ""
}

func (c *GenericConfig) getBool(key string) bool {
	if value, ok := c.configData[key].(bool); ok {
		return value
	}
	return false
}

func (c *GenericConfig) getInt(key string) int {
	if value, ok := c.configData[key].(int); ok {
		return value
	}
	return 0
}

func (c *GenericConfig) getInt64(key string) int64 {
	if value, ok := c.configData[key].(int64); ok {
		return value
	}
	return 0
}

func (c *GenericConfig) getFloat64(key string) float64 {
	if value, ok := c.configData[key].(float64); ok {
		return value
	}
	return 0.0
}

func (c *GenericConfig) getStringSlice(key string) []string {
	if value, ok := c.configData[key].([]string); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getIntSlice(key string) []int {
	if value, ok := c.configData[key].([]int); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getBoolSlice(key string) []bool {
	if value, ok := c.configData[key].([]bool); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getFloat64Slice(key string) []float64 {
	if value, ok := c.configData[key].([]float64); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getFloat32Slice(key string) []float32 {
	if value, ok := c.configData[key].([]float32); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getInt64Slice(key string) []int64 {
	if value, ok := c.configData[key].([]int64); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getStringMap(key string) map[string]string {
	if value, ok := c.configData[key].(map[string]string); ok {
		return value
	}
	return nil
}

func (c *GenericConfig) getInterfaceMap(key string) map[string]interface{} {
	if value, ok := c.configData[key].(map[string]interface{}); ok {
		return value
	}
	return nil
}

type intSliceValue struct {
	value *[]int
}

func newIntSliceValue(val []int) *intSliceValue {
	return &intSliceValue{value: &val}
}

func (s *intSliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *intSliceValue) Set(val string) error {
	var intSlice []int
	for _, v := range strings.Split(val, ",") {
		var intValue int
		if _, err := fmt.Sscanf(v, "%d", &intValue); err != nil {
			return ErrorWrapper(err, 0, "")
		}
		intSlice = append(intSlice, intValue)
	}
	*s.value = intSlice
	return nil
}

type stringSliceValue struct {
	value *[]string
}

func newStringSliceValue(val []string) *stringSliceValue {
	return &stringSliceValue{value: &val}
}

func (s *stringSliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *stringSliceValue) Set(val string) error {
	*s.value = strings.Split(val, ",")
	return nil
}

type boolSliceValue struct {
	value *[]bool
}

func newBoolSliceValue(val []bool) *boolSliceValue {
	return &boolSliceValue{value: &val}
}

func (s *boolSliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *boolSliceValue) Set(val string) error {
	var boolSlice []bool
	for _, v := range strings.Split(val, ",") {
		var boolValue bool
		if _, err := fmt.Sscanf(v, "%t", &boolValue); err != nil {
			return ErrorWrapper(err, 0, "")
		}
		boolSlice = append(boolSlice, boolValue)
	}
	*s.value = boolSlice
	return nil
}

type float64SliceValue struct {
	value *[]float64
}

func newFloat64SliceValue(val []float64) *float64SliceValue {
	return &float64SliceValue{value: &val}
}

func (s *float64SliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *float64SliceValue) Set(val string) error {
	var float64Slice []float64
	for _, v := range strings.Split(val, ",") {
		var float64Value float64
		if _, err := fmt.Sscanf(v, "%f", &float64Value); err != nil {
			return ErrorWrapper(err, 0, "")
		}
		float64Slice = append(float64Slice, float64Value)
	}
	*s.value = float64Slice
	return nil
}

type float32SliceValue struct {
	value *[]float32
}

func newFloat32SliceValue(val []float32) *float32SliceValue {
	return &float32SliceValue{value: &val}
}

func (s *float32SliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *float32SliceValue) Set(val string) error {
	var float32Slice []float32
	for _, v := range strings.Split(val, ",") {
		var float32Value float32
		if _, err := fmt.Sscanf(v, "%f", &float32Value); err != nil {
			return ErrorWrapper(err, 0, "")
		}
		float32Slice = append(float32Slice, float32Value)
	}
	*s.value = float32Slice
	return nil
}

type int64SliceValue struct {
	value *[]int64
}

func newInt64SliceValue(val []int64) *int64SliceValue {
	return &int64SliceValue{value: &val}
}

func (s *int64SliceValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *int64SliceValue) Set(val string) error {
	var int64Slice []int64
	for _, v := range strings.Split(val, ",") {
		var int64Value int64
		if _, err := fmt.Sscanf(v, "%d", &int64Value); err != nil {
			return ErrorWrapper(err, 0, "")
		}
		int64Slice = append(int64Slice, int64Value)
	}
	*s.value = int64Slice
	return nil
}

type stringMapValue struct {
	value *map[string]string
}

func newStringMapValue(val map[string]string) *stringMapValue {
	return &stringMapValue{value: &val}
}

func (s *stringMapValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *stringMapValue) Set(val string) error {
	*s.value = make(map[string]string)
	for _, v := range strings.Split(val, ",") {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid map item: %s", v)
		}
		(*s.value)[parts[0]] = parts[1]
	}
	return nil
}

type interfaceMapValue struct {
	value *map[string]interface{}
}

func newInterfaceMapValue(val map[string]interface{}) *interfaceMapValue {
	return &interfaceMapValue{value: &val}
}

func (s *interfaceMapValue) String() string {
	return fmt.Sprintf("%v", *s.value)
}

func (s *interfaceMapValue) Set(val string) error {
	*s.value = make(map[string]interface{})
	for _, v := range strings.Split(val, ",") {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid map item: %s", v)
		}
		(*s.value)[parts[0]] = parts[1]
	}
	return nil
}

func (c *GenericConfig) SetDefaults() {
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

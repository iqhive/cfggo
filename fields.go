package cfggo

import (
	"reflect"
	"strings"
	"sync"
)

// configNameCache caches the results of getConfigNameFromField.
var (
	configNameCache      = make(map[string]string)
	configNameCacheMutex sync.RWMutex
)

// getFieldKey returns a stable string key for a StructField, used for caching.
func getFieldKey(field reflect.StructField) string {
	return field.PkgPath + "." + field.Name + ":" + string(field.Tag)
}

// getConfigNameFromField returns the config map key for a struct field by
// inspecting struct tags in priority order: cfggo, cfg, config, json, Name.
func (c *Structure) getConfigNameFromField(field reflect.StructField) string {
	fieldKey := getFieldKey(field)

	configNameCacheMutex.RLock()
	if name, exists := configNameCache[fieldKey]; exists {
		configNameCacheMutex.RUnlock()
		return name
	}
	configNameCacheMutex.RUnlock()

	configVarName := field.Tag.Get("cfggo")
	if configVarName == "" {
		configVarName = field.Tag.Get("cfg")
	}
	if configVarName == "" {
		configVarName = field.Tag.Get("config")
	}
	if configVarName == "" {
		configVarName = field.Tag.Get("json")
	}
	if configVarName == "" {
		configVarName = field.Name
	}

	// Strip options after a comma (e.g. `json:"name,omitempty"`).
	if idx := strings.Index(configVarName, ","); idx != -1 {
		configVarName = configVarName[:idx]
	}

	configNameCacheMutex.Lock()
	configNameCache[fieldKey] = configVarName
	configNameCacheMutex.Unlock()

	return configVarName
}

// structToRecurse decides whether a struct field should be treated as a nested
// config group and, if so, returns the struct Value to recurse into
//
// It accepts both struct fields (e.g. `DB Database`) and pointer-to-struct
// fields (e.g. `DB *Database`). A nil pointer sub-struct is allocated in place
// (when the field is settable) so the func-typed accessor fields it contains
// can be wired up and read; this is why pointer sub-structs are non-nil after
// Init. The embedded cfggo.Structure is never treated as a config group
func structToRecurse(field reflect.StructField, fieldValue reflect.Value) (reflect.Value, bool) {
	if field.Anonymous && field.Type == reflect.TypeOf(Structure{}) {
		return reflect.Value{}, false
	}

	switch field.Type.Kind() {
	case reflect.Struct:
		return fieldValue, true
	case reflect.Ptr:
		elem := field.Type.Elem()
		if elem.Kind() != reflect.Struct || elem == reflect.TypeOf(Structure{}) {
			return reflect.Value{}, false
		}
		if fieldValue.IsNil() {
			if !fieldValue.CanSet() {
				return reflect.Value{}, false
			}
			fieldValue.Set(reflect.New(elem))
		}
		return fieldValue.Elem(), true
	}
	return reflect.Value{}, false
}

// structTypeToRecurse mirrors structToRecurse for type-only walks where there
// is no value to allocate, returning the struct type to recurse into
// This resolves pointer-to-struct fields to their element type
func structTypeToRecurse(field reflect.StructField) (reflect.Type, bool) {
	if field.Anonymous && field.Type == reflect.TypeOf(Structure{}) {
		return nil, false
	}
	t := field.Type
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && t != reflect.TypeOf(Structure{}) {
		return t, true
	}
	return nil, false
}

// setupConfigData walks the parent struct and populates c.configData with
// zero/default values for each config field.
func (c *Structure) setupConfigData() {
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		c.logWarnf("SetupConfigData: expected struct, got %v", v.Kind())
		return
	}

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
				continue
			}

			fullKey := configVarName
			if prefix != "" {
				fullKey = prefix + "." + configVarName
			}

			if sv, ok := structToRecurse(field, fieldValue); ok {
				processStruct(sv, sv.Type(), fullKey)
				continue
			}

			// Only zero-arg+single-return funcs (func() T) are accessors
			// skip anything else like a structs own func() field
			if !isAccessorFunc(fieldValue) {
				continue
			}

			if fieldValue.IsNil() || !fieldValue.CanInterface() {
				if err := c.set(fullKey, reflect.Zero(fieldValue.Type().Out(0)).Interface()); err != nil {
					c.logWarnf("Failed to set default value for %s: %v", fullKey, err)
				}
				continue
			}

			if err := c.set(fullKey, fieldValue.Call(nil)[0].Interface()); err != nil {
				c.logWarnf("Failed to set value for %s: %v", fullKey, err)
			}
		}
	}

	processStruct(v, v.Type(), "")
}

// isAccessorFunc reports whether v is a cfggo config accessor: a func with no
// parameters and exactly one return value - func() T)
func isAccessorFunc(v reflect.Value) bool {
	if v.Kind() != reflect.Func {
		return false
	}
	ft := v.Type()
	return ft.NumIn() == 0 && ft.NumOut() == 1
}

// setDefaultsFromTags reads `default:"..."` struct tags and applies them to
// any func fields that are still nil after setupConfigData.
func (c *Structure) setDefaultsFromTags() {
	if c.defaultsAlreadySet {
		return
	}
	c.defaultsAlreadySet = true

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		c.logWarnf("SetDefaults: expected struct, got %v", v.Kind())
		return
	}

	// Walk recursively so `default` tags on func fields inside nested
	// (non-embedded) structs are honoured too, keyed by the same dotted keys
	// that setupConfigData uses.
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
				continue
			}

			fullKey := configVarName
			if prefix != "" {
				fullKey = prefix + "." + configVarName
			}

			if sv, ok := structToRecurse(field, fieldValue); ok {
				processStruct(sv, sv.Type(), fullKey)
				continue
			}

			if !isAccessorFunc(fieldValue) || !fieldValue.IsNil() {
				continue
			}

			if defaultStr, ok := field.Tag.Lookup("default"); ok && defaultStr != "" {
				dv := &dynamicVar{
					config: c,
					name:   fullKey,
					want:   fieldValue.Type().Out(0),
				}
				if err := dv.Set(defaultStr); err != nil {
					c.logWarnf("SetDefaults: could not parse default value for field %s: %v", field.Name, err)
				}
			}
		}
	}

	processStruct(v, v.Type(), "")
}

// replaceConfigFuncs wires each func field in the parent struct to read from
// the config map so that hot reloading is transparent to callers.
func (c *Structure) replaceConfigFuncs() {
	// This mutates the parent struct func fields via reflect.Value.Set, so it
	// must hold the write lock: a read lock would let two concurrent callers
	// (eg overlapping reloads) write the same field at once
	c.configMutex.Lock()
	defer c.configMutex.Unlock()

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			c.log().Warn("ReplaceConfigFuncs: received nil pointer")
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		c.logWarnf("ReplaceConfigFuncs: expected struct or pointer to struct, got %v", v.Kind())
		return
	}

	// Walk the struct recursively so func fields inside nested (non-embedded)
	// structs are wired up too, using the same dotted keys that
	// setupConfigData populated the config map with.
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
				continue
			}

			fullKey := configVarName
			if prefix != "" {
				fullKey = prefix + "." + configVarName
			}

			if sv, ok := structToRecurse(field, fieldValue); ok {
				processStruct(sv, sv.Type(), fullKey)
				continue
			}

			if !isAccessorFunc(fieldValue) {
				continue
			}

			if _, exists := c.configData[fullKey]; !exists {
				c.logErrorf("Missing configData value for key %s", fullKey)
				continue
			}

			if !fieldValue.CanSet() {
				continue
			}

			func(key string, outType reflect.Type) {
				fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(_ []reflect.Value) []reflect.Value {
					c.configMutex.RLock()
					defer c.configMutex.RUnlock()
					raw := c.configData[key]
					// A nil value (eg an explicit JSON null, an unset interface{}
					// field, or a failed conversion gives an invalid reflect.Value,
					// so fall back to the typed zero value
					if raw == nil {
						return []reflect.Value{reflect.Zero(outType)}
					}
					rv := reflect.ValueOf(raw)
					if !rv.Type().AssignableTo(outType) {
						if rv.Type().ConvertibleTo(outType) {
							rv = rv.Convert(outType)
						} else {
							return []reflect.Value{reflect.Zero(outType)}
						}
					}
					return []reflect.Value{rv}
				}))
			}(fullKey, fieldValue.Type().Out(0))
		}
	}

	processStruct(v, v.Type(), "")
}

// createFlags registers a flag for every key in the config map.
func (c *Structure) createFlags() {
	for key, value := range c.configData {
		configDescription := c.GetHelpTag(key)
		c.NewFlag(key, value, configDescription)
	}
}

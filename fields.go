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
		Logger.Warnf("SetupConfigData: expected struct, got %v", v.Kind())
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

			if fieldValue.Kind() == reflect.Struct {
				processStruct(fieldValue, field.Type, fullKey)
			} else if fieldValue.Kind() == reflect.Func && fieldValue.IsNil() {
				if err := c.set(fullKey, reflect.Zero(fieldValue.Type().Out(0)).Interface()); err != nil {
					Logger.Warnf("Failed to set default value for %s: %v", fullKey, err)
				}
			} else if fieldValue.Kind() == reflect.Func {
				if fieldValue.CanInterface() {
					if err := c.set(fullKey, fieldValue.Call(nil)[0].Interface()); err != nil {
						Logger.Warnf("Failed to set value for %s: %v", fullKey, err)
					}
				} else {
					if err := c.set(fullKey, reflect.Zero(fieldValue.Type().Out(0)).Interface()); err != nil {
						Logger.Warnf("Failed to set default value for %s: %v", fullKey, err)
					}
				}
			}
		}
	}

	processStruct(v, v.Type(), "")
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

		if defaultStr, ok := field.Tag.Lookup("default"); ok && defaultStr != "" {
			dv := &dynamicVar{
				config: c,
				name:   configVarName,
				want:   fieldValue.Type().Out(0),
			}
			if err := dv.Set(defaultStr); err != nil {
				Logger.Warnf("SetDefaults: could not parse default value for field %s: %v", field.Name, err)
			}
		}
	}
}

// replaceConfigFuncs wires each func field in the parent struct to read from
// the config map so that hot reloading is transparent to callers.
func (c *Structure) replaceConfigFuncs() {
	configMutex.RLock()
	defer configMutex.RUnlock()

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			Logger.Warn("ReplaceConfigFuncs: received nil pointer")
			return
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		Logger.Warnf("ReplaceConfigFuncs: expected struct or pointer to struct, got %v", v.Kind())
		return
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if fieldValue.Kind() != reflect.Func {
			continue
		}

		configVarName := c.getConfigNameFromField(field)
		if configVarName == "" || configVarName == "-" {
			continue
		}

		if _, exists := c.configData[configVarName]; !exists {
			Logger.Errorf("Missing configData value for key %s", configVarName)
			continue
		}

		if !fieldValue.CanSet() {
			continue
		}

		func(key string) {
			fieldValue.Set(reflect.MakeFunc(fieldValue.Type(), func(_ []reflect.Value) []reflect.Value {
				configMutex.RLock()
				defer configMutex.RUnlock()
				return []reflect.Value{reflect.ValueOf(c.configData[key])}
			}))
		}(configVarName)
	}
}

// createFlags registers a flag for every key in the config map.
func (c *Structure) createFlags() {
	for key, value := range c.configData {
		configDescription := c.GetHelpTag(key)
		c.NewFlag(key, value, configDescription)
	}
}

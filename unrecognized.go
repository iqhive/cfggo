package cfggo

import (
	"reflect"
)

var commandLineOnlyFlags []string

func IgnoreFlags(flags ...string) {
	commandLineOnlyFlags = append(commandLineOnlyFlags, flags...)
}

func (c *GenericConfig) CheckUnrecognizedItems(s interface{}) {
	allKeys := c.getAllKeys()
	recognizedKeys := make(map[string]bool)

	v := reflect.ValueOf(s)

	// Ensure we're working with the struct value, not a pointer
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		Logger.Warn("CheckUnrecognizedItems: expected struct, got %v", v.Kind())
		return
	}

	c.walkStructFieldsWithKeys(v, "", recognizedKeys)

	// Add exceptions for command line only flags
	for _, flag := range commandLineOnlyFlags {
		recognizedKeys[flag] = true
	}

	for _, key := range allKeys {
		if !recognizedKeys[key] {
			Logger.Warn("Warning: Unrecognized %s item '%s' found", c.name, key)
		}
	}
}

func (c *GenericConfig) walkStructFieldsWithKeys(v reflect.Value, prefix string, recognizedKeys map[string]bool) {
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		tag := field.Tag.Get("config")
		if tag == "" {
			tag = field.Name
		}

		if prefix != "" {
			tag = prefix + "." + tag
		}

		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			// For embedded structs, continue walking without adding a prefix
			c.walkStructFieldsWithKeys(fieldValue, prefix, recognizedKeys)
		} else if fieldValue.Kind() == reflect.Struct {
			// For non-embedded structs, continue walking with the current tag as prefix
			c.walkStructFieldsWithKeys(fieldValue, tag, recognizedKeys)
		} else {
			recognizedKeys[tag] = true
		}
	}
}

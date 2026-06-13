package cfggo

import (
	"reflect"
)

var commandLineOnlyFlags []string

func IgnoreFlags(flags ...string) {
	commandLineOnlyFlags = append(commandLineOnlyFlags, flags...)
}

func (c *Structure) CheckUnrecognizedItems(s interface{}) {
	if c.parent == nil {
		c.InitSelf()
	}

	allKeys := c.getAllKeys()
	recognizedKeys := make(map[string]bool)

	v := reflect.ValueOf(s)

	// Ensure we're working with the struct value, not a pointer
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		c.logWarnf("CheckUnrecognizedItems: expected struct, got %v", v.Kind())
		return
	}

	c.walkStructFieldsWithKeys(v, "", recognizedKeys)

	// Add exceptions for command line only flags
	for _, flag := range commandLineOnlyFlags {
		recognizedKeys[flag] = true
	}

	for _, key := range allKeys {
		if !recognizedKeys[key] {
			c.logWarnf("Warning: Unrecognized %s item '%s' found", c.name, key)
		}
	}
}

func (c *Structure) walkStructFieldsWithKeys(v reflect.Value, prefix string, recognizedKeys map[string]bool) {
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

		// Skip the embedded cfggo.Structure: it holds internal bookkeeping
		// including a *flag.FlagSet, that must not be walked as config keys
		if field.Anonymous && field.Type == reflect.TypeOf(Structure{}) {
			continue
		}

		tag := field.Tag.Get("cfggo")
		if tag == "" {
			tag = field.Tag.Get("cfg")
		}
		if tag == "" {
			tag = field.Tag.Get("config")
		}
		if tag == "" {
			tag = field.Tag.Get("json")
		}
		if tag == "" {
			tag = field.Name
		}

		if prefix != "" {
			tag = prefix + "." + tag
		}

		// Resolve struct and pointer-to-struct fields to the struct value to
		// recurse into. A nil pointer sub-struct is walked via a temporary zero
		// value purely to collect its key names (no mutation of the input)
		structValue := fieldValue
		isStruct := false
		if field.Type.Kind() == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
			if fieldValue.IsNil() {
				structValue = reflect.New(field.Type.Elem()).Elem()
			} else {
				structValue = fieldValue.Elem()
			}
			isStruct = true
		} else if field.Type.Kind() == reflect.Struct {
			isStruct = true
		}

		if field.Anonymous && isStruct {
			// For embedded structs, continue walking without adding a prefix
			c.walkStructFieldsWithKeys(structValue, prefix, recognizedKeys)
		} else if isStruct {
			// For non-embedded structs, continue walking with the current tag as prefix
			c.walkStructFieldsWithKeys(structValue, tag, recognizedKeys)
		} else {
			recognizedKeys[tag] = true
		}
	}
}

package cfggo

import (
	"reflect"
	"sort"
)

// fieldInfo is the precomputed metadata for a single leaf (non-struct) field of
// the parent configuration struct. It is built once during Init so that
// help-tag lookups, key-recognition checks, and the config reference do not
// repeatedly walk the struct with reflection on every call
type fieldInfo struct {
	// Key is the dotted configuration key (eg "db.host")
	Key string
	// Help is the `help` struct tag, if any
	Help string
	// DefaultTag is the `default` struct tag value, if present
	DefaultTag string
	// HasDefault reports whether a `default` tag was present
	HasDefault bool
	// Type is the accessor's return type (the T in func() T) for accessor
	// fields, and nil otherwise
	Type reflect.Type
	// IsAccessor reports whether the field is a cfggo accessor (func() T) and
	// therefore backs a real configuration value
	IsAccessor bool
}

// buildFieldMeta walks the parent struct once and records leaf field metadata
// into c.fields (keyed by dotted config key) and the keys of `-`-tagged fields
// into c.ignoredFields - It is idempotent
func (c *Structure) buildFieldMeta() {
	if c.fields != nil {
		return
	}
	c.fields = make(map[string]fieldInfo)
	c.ignoredFields = make(map[string]bool)

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	c.walkFieldMeta(v.Type(), "")
}

func (c *Structure) walkFieldMeta(t reflect.Type, prefix string) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && field.Type == reflect.TypeOf(Structure{}) {
			continue
		}

		configVarName := c.getConfigNameFromField(field)

		// A field tagged "-" is excluded from config. Record the dotted
		// Go-field-name key so shouldIgnoreField can match the same key callers
		// would use for it (mirrors the historical behaviour)
		if configVarName == "-" {
			fieldName := field.Name
			if prefix != "" {
				fieldName = prefix + "." + fieldName
			}
			c.ignoredFields[fieldName] = true
			continue
		}
		if configVarName == "" {
			continue
		}

		fullKey := configVarName
		if prefix != "" {
			fullKey = prefix + "." + configVarName
		}

		if st, ok := structTypeToRecurse(field); ok {
			c.walkFieldMeta(st, fullKey)
			continue
		}

		info := fieldInfo{
			Key:  fullKey,
			Help: field.Tag.Get("help"),
		}
		if dv, ok := field.Tag.Lookup("default"); ok {
			info.DefaultTag = dv
			info.HasDefault = true
		}
		ft := field.Type
		if ft.Kind() == reflect.Func && ft.NumIn() == 0 && ft.NumOut() == 1 {
			info.IsAccessor = true
			info.Type = ft.Out(0)
		}
		c.fields[fullKey] = info
	}
}

// unrecognizedKeys returns the configuration keys currently present in the
// config map that are not backed by a struct field (and are not exempted via
// IgnoreFlags). These are almost always typos in a config file or environment
// variable. The result is sorted
func (c *Structure) unrecognizedKeys() []string {
	c.configMutex.RLock()
	keys := make([]string, 0, len(c.configData))
	for k := range c.configData {
		keys = append(keys, k)
	}
	c.configMutex.RUnlock()

	ignore := make(map[string]bool, len(commandLineOnlyFlags))
	for _, f := range commandLineOnlyFlags {
		ignore[f] = true
	}

	var out []string
	for _, k := range keys {
		if _, ok := c.fields[k]; ok {
			continue
		}
		if ignore[k] {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

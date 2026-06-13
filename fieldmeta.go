package cfggo

import (
	"reflect"
	"sort"
)

// fieldInfo is the precomputed metadata for a single leaf (non-struct) field of
// the parent configuration struct. It is built once per struct type (see
// structPlan) so that help-tag lookups, key-recognition checks, and the config
// reference do not repeatedly walk the struct with reflection on every call
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
	// IsSecret reports whether the field is tagged `secret:"true"`. Secret
	// values are masked in the human-readable / diagnostic outputs (Explain,
	// String, Diagnose, ConfigReference, Report) so a config dump pasted into a
	// log or bug report does not leak credentials. It does NOT affect Save /
	// GetJSONBytes, which must persist real values for round-tripping
	IsSecret bool
}

// maskedValue is the placeholder shown in human-readable / diagnostic output in
// place of a value whose field is tagged `secret:"true"`
const maskedValue = "****"

// isSecretKey reports whether the field backing key is tagged `secret:"true"`
func (c *Structure) isSecretKey(key string) bool {
	if c.plan == nil {
		return false
	}
	if leaf, ok := c.plan.byKey[key]; ok {
		return leaf.info.IsSecret
	}
	return false
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
		if c.plan != nil {
			if _, ok := c.plan.byKey[k]; ok {
				continue
			}
		}
		if ignore[k] {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

package cfggo

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
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

// provenanceValue returns the value to embed in a validation error's
// provenance for key: the real value for ordinary fields, and the masked
// placeholder for secret-tagged fields so error messages (which are logged and
// surfaced in diagnostics) never carry the sensitive value
func (c *Structure) provenanceValue(key string, value interface{}) interface{} {
	if c.isSecretKey(key) {
		return maskedValue
	}
	return value
}

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
// WithIgnoreKeys). These are almost always typos in a config file or
// environment variable. The result is sorted
func (c *Structure) unrecognizedKeys() []string {
	c.configMutex.RLock()
	keys := make([]string, 0, len(c.configData))
	for k := range c.configData {
		keys = append(keys, k)
	}
	c.configMutex.RUnlock()

	var out []string
	for _, k := range keys {
		if c.plan != nil {
			if leaf, ok := c.plan.byKey[k]; ok && leaf.info.IsAccessor {
				continue
			}
		}
		if c.ignoredKeys[k] {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Structure) suggestKey(key string) string {
	if c.plan == nil || len(c.plan.byKey) == 0 {
		return ""
	}
	best := ""
	bestDistance := 0
	for candidate, leaf := range c.plan.byKey {
		if !leaf.info.IsAccessor {
			continue
		}
		distance := editDistance(key, candidate)
		if best == "" || distance < bestDistance || (distance == bestDistance && candidate < best) {
			best = candidate
			bestDistance = distance
		}
	}
	if best == "" || bestDistance > suggestionDistanceLimit(key, best) {
		return ""
	}
	return best
}

func (c *Structure) unknownKeyMessage(key string) string {
	if suggestion := c.suggestKey(key); suggestion != "" {
		return fmt.Sprintf("%s (did you mean %s?)", key, suggestion)
	}
	return key
}

func (c *Structure) didYouMeanSuffix(key string) string {
	if suggestion := c.suggestKey(key); suggestion != "" {
		return fmt.Sprintf(" (did you mean %q?)", suggestion)
	}
	return ""
}

func (c *Structure) formatUnknownKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = c.unknownKeyMessage(key)
	}
	return strings.Join(out, ", ")
}

func suggestionDistanceLimit(a, b string) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	switch {
	case n <= 4:
		return 2
	case n <= 8:
		return 2
	default:
		return 3
	}
}

func editDistance(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = minInt(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func minInt(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

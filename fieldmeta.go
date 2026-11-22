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
	defer c.configMutex.RUnlock()
	return c.unrecognizedKeysLocked()
}

// unrecognizedKeysLocked is unrecognizedKeys for callers that already hold
// configMutex
func (c *Structure) unrecognizedKeysLocked() []string {
	var out []string
	for k := range c.configData {
		if c.plan != nil {
			if leaf, ok := c.plan.byKey[k]; ok && leaf.info.IsAccessor {
				continue
			}
		}
		if _, ok := c.extraKeys[k]; ok {
			continue
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
		distance := editDistanceLimited(key, candidate, maxSuggestionDistance)
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

// maxSuggestionDistance is the largest distance suggestionDistanceLimit can
// return, so editDistanceLimited can cut off work beyond it
const maxSuggestionDistance = 3

// editDistanceLimited returns the Levenshtein distance between a and b when it
// is at most limit, and limit+1 otherwise.
//
// suggestKey runs a distance computation for every (unknown key, known key)
// pair, and the unknown keys come from a loaded document that can be filled
// with arbitrary strings, so the full quadratic matrix would let a large
// hostile file cost minutes of CPU at startup. Two properties of edit distance
// bound the work instead:
//
//   - d(a, b) >= |len(a) - len(b)|, so a pair whose lengths differ by more
//     than limit is rejected without allocating anything. This also means the
//     bottom-right cell is always inside the final row's band, so the walk
//     never reads a cell it did not write.
//   - every cell of the dynamic program outside a diagonal band of width
//     2*limit+1 is greater than limit, so only the band is filled (with a
//     sentinel written on each edge so the next row never reads a stale
//     cell), and the first row whose smallest value exceeds the limit ends
//     the walk.
//
// Distances up to limit are exact, which is all the suggestion logic
// compares: larger distances only ever mean "not a match" and are reported
// as the single value limit+1. The walk is by bytes, matching editDistance,
// so the two agree on every pair.
func editDistanceLimited(a, b string, limit int) int {
	if a == b {
		return 0
	}
	exceeds := limit + 1
	if abs(len(a)-len(b)) > limit {
		return exceeds
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		if j <= limit {
			prev[j] = j
		} else {
			prev[j] = exceeds
		}
	}
	for i := 1; i <= len(a); i++ {
		lo := i - limit
		if lo < 1 {
			lo = 1
		}
		hi := i + limit
		if hi > len(b) {
			hi = len(b)
		}
		// The j == 0 column is exact while it is inside the band, and a
		// sentinel once i outruns the limit
		if i <= limit {
			curr[0] = i
		} else {
			curr[0] = exceeds
		}
		// Sentinel edges, so the next row's reads at lo-1 and hi+1 never see
		// a cell this row did not write
		if lo > 1 {
			curr[lo-1] = exceeds
		}
		if hi < len(b) {
			curr[hi+1] = exceeds
		}
		rowMin := curr[0]
		for j := lo; j <= hi; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d := prev[j] + 1
			if v := curr[j-1] + 1; v < d {
				d = v
			}
			if v := prev[j-1] + cost; v < d {
				d = v
			}
			curr[j] = d
			if d < rowMin {
				rowMin = d
			}
		}
		if rowMin > limit {
			// Every remaining row can only be further away
			return exceeds
		}
		prev, curr = curr, prev
	}
	if prev[len(b)] > limit {
		return exceeds
	}
	return prev[len(b)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// editDistance is the plain, unbounded Levenshtein implementation. It is the
// oracle editDistanceLimited is verified against in
// TestEditDistanceLimitedMatchesPlain; the suggestion paths use the bounded
// variant so a document full of long unknown keys cannot make suggestion
// computation quadratic in the document size
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

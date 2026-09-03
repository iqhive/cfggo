package cfggo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	iconvert "github.com/iqhive/cfggo/internal/convert"
)

func (c *Structure) loadConfig(alreadyLocked bool) error {
	if c.configHandler == nil {
		return c.WrapError(ErrNoHandler, 400, "")
	}

	src := c.handlerSource()
	data, err := c.configHandler.LoadConfig()
	if err != nil {
		return c.WrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument,
			"failed to load configuration from %s source", src)
	}

	return c.loadJSONConfigFromBytes(data, alreadyLocked)
}

func (c *Structure) loadJSONConfigFromBytes(data []byte, alreadyLocked bool) error {
	if len(data) == 0 {
		c.log().Debug("cfggo: empty or nil config data provided")
		return nil
	}

	src := c.handlerSource()
	rawConfig, err := decodeJSONConfig(data)
	if err != nil {
		// A malformed config file is a hard error by default: starting with
		// silently-ignored config is usually worse than failing loudly.
		// Callers that want best-effort loading opt in via WithLenientLoad
		if c.lenient {
			c.log().Warn("cfggo: ignoring malformed configuration JSON", "err", err)
			return nil
		}
		return c.WrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument,
			"failed to parse configuration JSON from %s source", src)
	}

	// Flatten first so a key given both flat ("db.host") and nested
	// ("db": {"host": ...}) is detected instead of being applied in whichever
	// order the map happens to iterate
	entries, duplicates := c.flattenLoadedConfig(rawConfig)
	if len(duplicates) > 0 {
		dupErr := fmt.Errorf("duplicate configuration keys (given both flat and nested): %s", strings.Join(duplicates, ", "))
		if !c.lenient {
			return c.WrapError(wrapKind(ErrSource, dupErr), ErrCodeInvalidArgument,
				"failed to parse configuration JSON from %s source", src)
		}
		c.log().Warn("cfggo: duplicate configuration keys; the first in key order wins", "source", src, "keys", duplicates)
	}

	if !alreadyLocked {
		c.configMutex.Lock()
		defer c.configMutex.Unlock()
	}

	var setErrs []error
	for _, entry := range entries {
		fullKey, value := entry.key, entry.value
		{
			if value == nil {
				// An explicit JSON null clears the value. For a known key keep
				// the typed zero value so accessors and flags stay correctly
				// typed; otherwise store a bare nil
				if existing, ok := c.configData[fullKey]; ok && existing != nil {
					c.configData[fullKey] = reflect.Zero(reflect.TypeOf(existing)).Interface()
				} else {
					c.configData[fullKey] = nil
				}
				c.recordSourceLocked(fullKey, src)
				c.recordLoadedLocked(fullKey)
				continue
			}

			// All type coercion is handled by the unified converter inside c.set.
			if err := c.set(fullKey, value); err != nil {
				// Conversion errors quote the raw input, so redact them for
				// secret-tagged fields: the error is logged and joined into the
				// returned load error, and must never carry the credential
				if c.isSecretKey(fullKey) {
					err = fmt.Errorf("invalid value (redacted: field is secret)")
				}
				c.log().Warn("cfggo: error setting config key from source", "key", fullKey, "source", src, "err", err)
				setErrs = append(setErrs, fmt.Errorf("key %q from %s: %w", fullKey, src, err))
				continue
			}
			c.recordSourceLocked(fullKey, src)
			c.recordLoadedLocked(fullKey)
		}
	}

	// Type-coercion failures (eg a string where an int is expected) are also
	// surfaced as a load error in strict mode so callers learn their config
	// file does not match the struct
	if len(setErrs) > 0 && !c.lenient {
		return c.WrapError(wrapKind(ErrSource, errors.Join(setErrs...)), ErrCodeInvalidArgument,
			"failed to apply configuration values from %s source", src)
	}
	return nil
}

// loadedEntry is one flattened key/value pair of a configuration document
type loadedEntry struct {
	key   string
	value interface{}
}

// flattenLoadedConfig turns a decoded document into dotted-key entries in a
// deterministic (sorted) order. Nested objects are descended unless the key is
// an accessor that takes an object itself (func() map[string]T, func() Struct).
// A key that appears more than once (flat and nested spellings of the same
// path) is reported in duplicates; its first occurrence in key order is kept
func (c *Structure) flattenLoadedConfig(document map[string]interface{}) ([]loadedEntry, []string) {
	var entries []loadedEntry
	var duplicates []string
	seen := make(map[string]bool, len(document))
	var walk func(map[string]interface{}, string)
	walk = func(m map[string]interface{}, prefix string) {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := m[key]
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			// Respect fields tagged with "-".
			if c.shouldIgnoreField(fullKey) {
				continue
			}
			if nested, ok := value.(map[string]interface{}); ok && !c.isAccessorKey(fullKey) {
				walk(nested, fullKey)
				continue
			}
			if seen[fullKey] {
				duplicates = append(duplicates, fullKey)
				continue
			}
			seen[fullKey] = true
			entries = append(entries, loadedEntry{key: fullKey, value: value})
		}
	}
	walk(document, "")
	sort.Strings(duplicates)
	return entries, duplicates
}

// recordLoadedLocked remembers the value the configuration source supplied for
// key, so Save can write it back when the live value is later overridden by
// the environment or a flag. The caller must hold configMutex for writing
func (c *Structure) recordLoadedLocked(key string) {
	if c.loadedData == nil {
		c.loadedData = make(map[string]interface{})
	}
	c.loadedData[key] = cloneMutableInterface(c.configData[key])
}

// persistableDataLocked returns the values Save writes.
//
// Every key is written with its current value, except that a key whose current
// value came from the environment or a command-line flag is written with the
// value the configuration source last supplied for it, or its default when the
// source never mentioned it. Runtime overrides are re-applied from their own
// sources on every start; persisting them would freeze a one-off flag into the
// file and, worse, copy a secret injected through the environment onto disk.
// Values set programmatically (SourceSet) are persisted: Set is the API for
// changing the saved configuration. The caller must hold configMutex
func (c *Structure) persistableDataLocked() map[string]interface{} {
	out := make(map[string]interface{}, len(c.configData))
	for key, value := range c.configData {
		switch c.provenance[key] {
		case SourceEnv, SourceFlag:
			if loaded, ok := c.loadedData[key]; ok {
				out[key] = loaded
			} else if def, ok := c.defaultData[key]; ok {
				out[key] = def
			}
		default:
			out[key] = value
		}
	}
	return out
}

// persistableJSONBytes marshals the values Save writes (see
// persistableDataLocked), as opposed to GetJSONBytes which is the full live
// state including runtime overrides
func (c *Structure) persistableJSONBytes() ([]byte, error) {
	c.configMutex.RLock()
	data, err := json.Marshal(c.persistableDataLocked())
	c.configMutex.RUnlock()
	if err != nil {
		return nil, c.WrapError(wrapKind(ErrSource, err), ErrCodeInternal, "failed to marshal configuration to JSON")
	}
	return data, nil
}

// decodeJSONConfig parses a configuration document into a generic map. Numbers
// are decoded as json.Number and then normalised (see
// convert.NormalizeJSONNumbers) so integers beyond 2^53 are not silently
// rounded through float64 on their way to an int64/uint64 field, while
// ordinary values keep the float64 dynamic type encoding/json would produce.
// Like json.Unmarshal, trailing data after the top-level value is an error
func decodeJSONConfig(data []byte) (map[string]interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var rawConfig map[string]interface{}
	if err := dec.Decode(&rawConfig); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("invalid character after top-level value")
	}
	normalised, err := iconvert.NormalizeJSONNumbers(rawConfig)
	if err != nil {
		return nil, err
	}
	rawConfig, _ = normalised.(map[string]interface{})
	return rawConfig, nil
}

// startAutoSave launches, when WithAutoSave(ctx) was supplied, a single
// goroutine that saves the configuration once the context is cancelled. cfggo
// no longer installs a process-wide signal handler or calls os.Exit: the
// application owns its shutdown lifecycle and passes in a context (commonly one
// from signal.NotifyContext). When the context is never cancelled the goroutine
// simply lives for the lifetime of the program.
func (c *Structure) startAutoSave() {
	if !c.autoSave || c.autoSaveCtx == nil {
		return
	}
	ctx := c.autoSaveCtx
	stop := make(chan struct{})
	c.autoSaveStop = stop
	go func() {
		select {
		case <-ctx.Done():
			if err := c.SaveIfChanged(); err != nil {
				c.log().Error("cfggo: auto-save on context cancellation failed", "err", err)
			}
		case <-stop:
			// The Init that started this goroutine was reverted
		}
	}()
}

// Save writes the configuration through the configured ConfigHandler and
// clears the dirty flag. It is a no-op (returning nil) when no handler is
// configured.
//
// Save persists the durable layers: defaults, values from the configuration
// source, and values changed with Set. A key whose live value is a runtime
// override from the environment or a command-line flag is written with the
// value the source last supplied (or its default), never with the override, so
// a secret injected through the environment is not copied onto disk and a
// one-off flag is not frozen into the file. GetJSONBytes returns the full live
// state when that is what you need
func (c *Structure) Save() error {
	c.ensureInit()

	c.configMutex.RLock()
	version := c.changeVersion
	c.configMutex.RUnlock()

	if err := c.saveConfig(); err != nil {
		return err
	}
	c.clearChangedIf(version)
	return nil
}

// SaveIfChanged saves the configuration only when a value has been changed
// with Set since it was last saved, clearing the dirty flag on a successful
// save. Values applied from the file, the environment or flags never make the
// configuration dirty. It is the building block applications should call
// from their own shutdown path
func (c *Structure) SaveIfChanged() error {
	c.ensureInit()

	c.configMutex.RLock()
	changed := c.changed
	version := c.changeVersion
	c.configMutex.RUnlock()
	if !changed {
		return nil
	}

	c.log().Info("cfggo: saving changed configuration")
	if err := c.saveConfig(); err != nil {
		return err
	}

	c.configMutex.Lock()
	if c.changeVersion == version {
		c.changed = false
	}
	c.configMutex.Unlock()
	return nil
}

// GetJSONBytes marshals the current configuration to JSON. Unlike the
// human-readable dumps it writes real values (secrets are not masked), and
// unlike Save it includes runtime overrides from the environment and flags: it
// is the full live state. A marshalling failure is both logged and returned so
// callers can react to it instead of silently receiving nil
func (c *Structure) GetJSONBytes() ([]byte, error) {
	c.ensureInit()

	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	data, err := json.Marshal(c.configData)
	if err != nil {
		c.log().Error("cfggo: failed to marshal configuration to JSON", "err", err)
		return nil, c.WrapError(wrapKind(ErrSource, err), ErrCodeInternal, "failed to marshal configuration to JSON")
	}
	return data, nil
}

// String returns a human-readable, key-sorted dump of the configuration.
// Values for fields tagged `secret:"true"` are masked,
// so the output is safe to log or paste into a bug report
func (c *Structure) String() string {
	return c.renderHuman(false)
}

// GetHelpTag returns the `help` struct tag for the field backing key, or "" if
// the key is unknown or has no help tag. It reads from the metadata computed
// once during Init rather than re-walking the struct on every call
func (c *Structure) GetHelpTag(key string) string {
	c.ensureInit()
	return c.helpTag(key)
}

func (c *Structure) helpTag(key string) string {
	if c.plan != nil {
		if leaf, ok := c.plan.byKey[key]; ok {
			return leaf.info.Help
		}
	}
	c.configMutex.RLock()
	help := c.extraKeys[key]
	c.configMutex.RUnlock()
	return help
}

// shouldIgnoreField returns true when a config key corresponds to a struct
// field tagged with `cfggo:"-"` (or its aliases). It reads from the metadata
// computed once during Init()
func (c *Structure) shouldIgnoreField(key string) bool {
	return c.plan != nil && c.plan.ignored[key]
}

func (c *Structure) isAccessorKey(key string) bool {
	if c.plan == nil {
		return false
	}
	leaf, ok := c.plan.byKey[key]
	return ok && leaf.info.IsAccessor
}

func (c *Structure) saveConfig() error {
	if c.configHandler == nil {
		return nil
	}

	data, err := c.persistableJSONBytes()
	if err != nil {
		return err
	}

	if err := c.configHandler.SaveConfig(data); err != nil {
		return c.WrapError(wrapKind(ErrSource, err), 0, "")
	}

	return nil
}

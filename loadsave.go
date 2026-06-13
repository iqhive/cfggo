package cfggo

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
)

func (c *Structure) loadConfig(alreadyLocked bool) error {
	if c.configHandler == nil {
		return c.WrapError(ErrNoHandler, 400, "")
	}

	data, err := c.configHandler.LoadConfig()
	if err != nil {
		return c.WrapError(wrapKind(ErrSource, err), 0, "")
	}

	return c.loadJSONConfigFromBytes(data, alreadyLocked)
}

func (c *Structure) loadJSONConfigFromBytes(data []byte, alreadyLocked bool) error {
	if len(data) == 0 {
		c.log().Debug("cfggo: empty or nil config data provided")
		return nil
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		// A malformed config file is a hard error by default: starting with
		// silently-ignored config is usually worse than failing loudly.
		// Callers that want best-effort loading opt in via WithLenientLoad
		if c.lenient {
			c.log().Warn("cfggo: ignoring malformed configuration JSON", "err", err)
			return nil
		}
		return c.WrapError(wrapKind(ErrSource, err), ErrCodeInvalidArgument, "failed to parse configuration JSON")
	}

	if !alreadyLocked {
		c.configMutex.Lock()
		defer c.configMutex.Unlock()
	}

	src := c.handlerSource()

	var setErrs []error
	var processMap func(map[string]interface{}, string)
	processMap = func(m map[string]interface{}, prefix string) {
		for key, value := range m {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}

			// Nested JSON objects represent nested struct fields; recurse.
			if nested, ok := value.(map[string]interface{}); ok {
				processMap(nested, fullKey)
				continue
			}

			// Respect fields tagged with "-".
			if c.shouldIgnoreField(fullKey) {
				continue
			}

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
				continue
			}

			// All type coercion is handled by the unified converter inside c.set.
			if err := c.set(fullKey, value); err != nil {
				c.log().Warn("cfggo: error setting config key from source", "key", fullKey, "source", src, "err", err)
				setErrs = append(setErrs, fmt.Errorf("%s: %w", fullKey, err))
				continue
			}
			c.recordSourceLocked(fullKey, src)
		}
	}

	processMap(rawConfig, "")

	// Type-coercion failures (eg a string where an int is expected) are also
	// surfaced as a load error in strict mode so callers learn their config
	// file does not match the struct
	if len(setErrs) > 0 && !c.lenient {
		return c.WrapError(wrapKind(ErrSource, errors.Join(setErrs...)), ErrCodeInvalidArgument,
			"failed to apply configuration values")
	}
	return nil
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
	go func() {
		<-ctx.Done()
		if err := c.SaveIfChanged(); err != nil {
			c.log().Error("cfggo: auto-save on context cancellation failed", "err", err)
		}
	}()
}

// Save writes the current configuration through the configured ConfigHandler
// It is a no-op (returning nil) when no handler is configured
func (c *Structure) Save() error {
	c.ensureInit()
	return c.saveConfig()
}

// SaveIfChanged saves the configuration only when it has been modified since it
// was last loaded or saved, clearing the dirty flag on a successful save
// It is the building block applications should call from their own shutdown path
func (c *Structure) SaveIfChanged() error {
	c.ensureInit()

	c.configMutex.RLock()
	changed := c.changed
	c.configMutex.RUnlock()
	if !changed {
		return nil
	}

	c.log().Info("cfggo: saving changed configuration")
	if err := c.saveConfig(); err != nil {
		return err
	}

	c.configMutex.Lock()
	c.changed = false
	c.configMutex.Unlock()
	return nil
}

// CleanupSignalHandler is retained for backwards compatibility and now does
// nothing: cfggo no longer installs a process-wide signal handler for
// auto-save. Drive shutdown saves via the context passed to WithAutoSave, or
// call Save / SaveIfChanged explicitly
// Deprecated: this is a no-op and will be removed in a future version.
func CleanupSignalHandler() {}

func (c *Structure) GetJSONBytes() []byte {
	c.ensureInit()

	c.configMutex.RLock()
	defer c.configMutex.RUnlock()
	data, _ := json.Marshal(c.configData)
	return data
}

// String returns a human-readable, key-sorted dump of the configuration.
// Values for fields tagged `secret:"true"` are masked,
// so the output is safe to log or paste into a bug report
func (c *Structure) String() string {
	c.ensureInit()

	c.configMutex.RLock()
	keys := make([]string, 0, len(c.configData))
	values := make(map[string]string, len(c.configData))
	for key, value := range c.configData {
		keys = append(keys, key)
		if c.isSecretKey(key) {
			values[key] = maskedValue
		} else {
			values[key] = fmt.Sprintf("%v", value)
		}
	}
	c.configMutex.RUnlock()

	// Sort keys so the output is deterministic (handy for diffs and bug reports)
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.name + ":\n")
	tw := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for _, key := range keys {
		if helpTag := c.GetHelpTag(key); helpTag != "" {
			fmt.Fprintf(tw, "%s\t%s\t// %s\n", key, values[key], helpTag)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t\n", key, values[key])
		}
	}
	tw.Flush()
	return sb.String()
}

// GetHelpTag returns the `help` struct tag for the field backing key, or "" if
// the key is unknown or has no help tag. It reads from the metadata computed
// once during Init rather than re-walking the struct on every call
func (c *Structure) GetHelpTag(key string) string {
	c.ensureInit()
	if c.plan != nil {
		if leaf, ok := c.plan.byKey[key]; ok {
			return leaf.info.Help
		}
	}
	return ""
}

// shouldIgnoreField returns true when a config key corresponds to a struct
// field tagged with `cfggo:"-"` (or its aliases). It reads from the metadata
// computed once during Init()
func (c *Structure) shouldIgnoreField(key string) bool {
	return c.plan != nil && c.plan.ignored[key]
}

func (c *Structure) saveConfig() error {
	if c.configHandler == nil {
		return nil
	}

	c.configMutex.RLock()
	data, err := json.Marshal(c.configData)
	c.configMutex.RUnlock()
	if err != nil {
		return c.WrapError(wrapKind(ErrSource, err), 0, "")
	}

	if err := c.configHandler.SaveConfig(data); err != nil {
		return c.WrapError(wrapKind(ErrSource, err), 0, "")
	}

	return nil
}

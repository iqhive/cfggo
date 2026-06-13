package cfggo

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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
		c.log().Debug("loadJSONConfigFromBytes: empty or nil data provided")
		return nil
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		c.logErrorf("Failed to unmarshal JSON data: %v", err)
		return nil
	}

	if !alreadyLocked {
		c.configMutex.Lock()
		defer c.configMutex.Unlock()
	}

	src := c.handlerSource()

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
				c.logWarnf("Error setting config key %s: %v", fullKey, err)
				continue
			}
			c.recordSourceLocked(fullKey, src)
		}
	}

	processMap(rawConfig, "")
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
			c.logErrorf("cfggo: auto-save on context cancellation failed: %v", err)
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

func (c *Structure) String() string {
	c.ensureInit()

	var sb strings.Builder
	sb.WriteString(c.name + ":\n")
	maxKeyLen := 0
	maxValueLen := 0

	c.configMutex.RLock()
	values := make(map[string]string, len(c.configData))
	for key, value := range c.configData {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}
		valueStr := fmt.Sprintf("%v", value)
		values[key] = valueStr
		if len(valueStr) > maxValueLen {
			maxValueLen = len(valueStr)
		}
	}
	c.configMutex.RUnlock()

	for key, valueStr := range values {
		helpSpacer := strings.Repeat(" ", maxValueLen-len(valueStr))
		helpTag := c.GetHelpTag(key)
		if helpTag != "" {
			sb.WriteString(fmt.Sprintf("%*s: %v %s// %s\n", maxKeyLen, key, valueStr, helpSpacer, helpTag))
		} else {
			sb.WriteString(fmt.Sprintf("%*s: %v\n", maxKeyLen, key, valueStr))
		}
	}
	return sb.String()
}

func (c *Structure) GetHelpTag(key string) string {
	c.ensureInit()

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}

	// Walk recursively so `help` tags on func fields inside nested
	// (non-embedded) structs resolve against the same dotted keys that
	// setupConfigData/createFlags use
	var walk func(reflect.Type, string) (string, bool)
	walk = func(t reflect.Type, prefix string) (string, bool) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
				continue
			}

			configVarName := c.getConfigNameFromField(field)
			if configVarName == "" || configVarName == "-" {
				continue
			}

			fullKey := configVarName
			if prefix != "" {
				fullKey = prefix + "." + configVarName
			}

			if st, ok := structTypeToRecurse(field); ok {
				if help, ok := walk(st, fullKey); ok {
					return help, true
				}
				continue
			}

			if fullKey == key {
				return field.Tag.Get("help"), true
			}
		}
		return "", false
	}

	help, _ := walk(v.Type(), "")
	return help
}

// shouldIgnoreField returns true when a config key corresponds to a struct
// field tagged with `cfggo:"-"` (or its aliases).
func (c *Structure) shouldIgnoreField(key string) bool {
	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}

	var checkStruct func(reflect.Type, string) bool
	checkStruct = func(t reflect.Type, prefix string) bool {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Type == reflect.TypeOf(Structure{}) && field.Anonymous {
				continue
			}
			configVarName := c.getConfigNameFromField(field)

			// A field tagged "-" has no config name, so match the explicit key
			// a caller would use for it (its Go field name) under the dotted
			// prefix built from the ancestor structs' config names.
			if configVarName == "-" {
				fieldName := field.Name
				if prefix != "" {
					fieldName = prefix + "." + fieldName
				}
				if key == fieldName {
					return true
				}
				continue
			}

			// Recurse into nested structs (including pointer sub-structs) using
			// the config-name prefix so the keys here line up with those
			// produced by setupConfigData.
			if st, ok := structTypeToRecurse(field); ok {
				nestedPrefix := configVarName
				if prefix != "" {
					nestedPrefix = prefix + "." + configVarName
				}
				if checkStruct(st, nestedPrefix) {
					return true
				}
			}
		}
		return false
	}

	return checkStruct(v.Type(), "")
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

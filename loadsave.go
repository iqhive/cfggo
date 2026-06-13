package cfggo

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
)

var configsToSave []*Structure
var configsToSaveMutex sync.Mutex
var once sync.Once
var signalChannel chan os.Signal
var signalCleanupOnce sync.Once

func (c *Structure) loadConfig(alreadyLocked bool) error {
	if c.configHandler == nil {
		return c.WrapError(nil, 400, "configSource is nil")
	}

	data, err := c.configHandler.LoadConfig()
	if err != nil {
		return c.WrapError(err, 0, "")
	}

	c.setupConfigSaver()

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
		configMutex.Lock()
		defer configMutex.Unlock()
	}

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
				continue
			}

			// All type coercion is handled by the unified converter inside c.set.
			if err := c.set(fullKey, value); err != nil {
				c.logWarnf("Error setting config key %s: %v", fullKey, err)
			}
		}
	}

	processMap(rawConfig, "")
	return nil
}

func (c *Structure) setupConfigSaver() {
	if !c.autoSave {
		return
	}

	configsToSaveMutex.Lock()
	configsToSave = append(configsToSave, c)
	configsToSaveMutex.Unlock()

	once.Do(func() {
		signalChannel = make(chan os.Signal, 1)
		signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
		go func() {
			defer func() {
				signal.Stop(signalChannel)
				close(signalChannel)
			}()

			<-signalChannel

			// Snapshot the slice under the lock so a concurrent Init that is
			// still appending cannot race with this iteration.
			configsToSaveMutex.Lock()
			snapshot := make([]*Structure, len(configsToSave))
			copy(snapshot, configsToSave)
			configsToSaveMutex.Unlock()

			for _, config := range snapshot {
				configMutex.RLock()
				changed := config.changed
				configMutex.RUnlock()
				if changed {
					config.log().Info("Saving config before exit...")
					if err := config.saveConfig(); err != nil {
						config.logErrorf("Error saving configuration: %v", err)
					}
				}
			}
			os.Exit(0)
		}()
	})
}

// CleanupSignalHandler allows tests and applications to release the signal
// handler installed by setupConfigSaver.
func CleanupSignalHandler() {
	signalCleanupOnce.Do(func() {
		if signalChannel != nil {
			signal.Stop(signalChannel)
			close(signalChannel)
			signalChannel = nil
		}
	})
}

func (c *Structure) GetJSONBytes() []byte {
	if c.parent == nil {
		c.InitSelf()
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	data, _ := json.Marshal(c.configData)
	return data
}

func (c *Structure) String() string {
	if c.parent == nil {
		c.InitSelf()
	}

	var sb strings.Builder
	sb.WriteString(c.name + ":\n")
	maxKeyLen := 0
	maxValueLen := 0

	configMutex.RLock()
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
	configMutex.RUnlock()

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
	if c.parent == nil {
		c.InitSelf()
	}

	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		configVarName := c.getConfigNameFromField(field)
		if configVarName == key {
			return field.Tag.Get("help")
		}
	}
	return ""
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
			configVarName := c.getConfigNameFromField(field)

			if configVarName == "-" {
				fieldName := field.Name
				if prefix != "" {
					fieldName = prefix + "." + fieldName
				}
				if key == fieldName {
					return true
				}
			}

			if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(Structure{}) {
				nestedPrefix := field.Name
				if prefix != "" {
					nestedPrefix = prefix + "." + nestedPrefix
				}
				if checkStruct(field.Type, nestedPrefix) {
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

	configMutex.RLock()
	data, err := json.Marshal(c.configData)
	configMutex.RUnlock()
	if err != nil {
		return c.WrapError(err, 0, "")
	}

	if err := c.configHandler.SaveConfig(data); err != nil {
		return c.WrapError(err, 0, "")
	}

	return nil
}

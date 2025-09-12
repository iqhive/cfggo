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
	"time"
)

var configsToSave []*Structure
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

	// if c.configHandler.SaveConfig != nil {
	// 	// Logger.Debug("Setting up config saver")
	// }
	c.setupConfigSaver()

	return c.loadJSONConfigFromBytes(data, alreadyLocked)
}

func (c *Structure) loadJSONConfigFromBytes(data []byte, alreadyLocked bool) error {
	if len(data) == 0 {
		Logger.Debug("loadJSONConfigFromBytes: empty or nil data provided")
		return nil
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		// Log the error but don't fail the entire configuration process
		// This allows the system to continue with defaults when JSON is invalid
		Logger.Errorf("Failed to unmarshal JSON data: %v", err)
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

			switch v := value.(type) {
			case map[string]interface{}:
				// Process nested maps
				processMap(v, fullKey)

				// Special handling for nested time.Duration fields
				// Check if any nested fields need special handling
				for nestedKey, nestedValue := range v {
					nestedFullKey := fullKey + "." + nestedKey
					if existingVal, exists := c.configData[nestedFullKey]; exists {
						// Handle time.Duration in nested structures
						if _, isDuration := existingVal.(time.Duration); isDuration {
							if strVal, ok := nestedValue.(string); ok {
								if duration, err := time.ParseDuration(strVal); err == nil {
									c.configData[nestedFullKey] = duration
								} else {
									Logger.Warnf("Failed to parse duration for %s: %v", nestedFullKey, err)
								}
							} else if floatVal, ok := nestedValue.(float64); ok {
								c.configData[nestedFullKey] = time.Duration(int64(floatVal))
							}
						}
					}
				}
			default:
				// Special handling for known types
				if existingVal, exists := c.configData[fullKey]; exists {
					// Handle nil values
					if v == nil {
						c.configData[fullKey] = nil
						continue
					}

					// Handle time.Time conversion
					if _, isTime := existingVal.(time.Time); isTime {
						if strVal, ok := v.(string); ok {
							if t, err := time.Parse(time.RFC3339, strVal); err == nil {
								c.configData[fullKey] = t
								continue
							} else {
								Logger.Warnf("Failed to parse time for %s: %v", fullKey, err)
							}
						}
					}

					// Handle time.Duration conversion
					if _, isDuration := existingVal.(time.Duration); isDuration {
						if strVal, ok := v.(string); ok {
							if duration, err := time.ParseDuration(strVal); err == nil {
								c.configData[fullKey] = duration
								continue
							} else {
								Logger.Warnf("Failed to parse duration for %s: %v", fullKey, err)
							}
						} else if floatVal, ok := v.(float64); ok {
							// Handle numeric duration (assuming nanoseconds)
							c.configData[fullKey] = time.Duration(int64(floatVal))
							continue
						}
					}

					// Handle slice conversions
					if reflect.TypeOf(existingVal) != nil && reflect.TypeOf(existingVal).Kind() == reflect.Slice {
						// Handle empty slices
						if sliceVal, ok := v.([]interface{}); ok {
							// Convert slice based on the existing type
							switch existingVal.(type) {
							case []string:
								strSlice := make([]string, len(sliceVal))
								for i, item := range sliceVal {
									if item != nil {
										strSlice[i] = fmt.Sprintf("%v", item)
									}
								}
								c.configData[fullKey] = strSlice
								continue
							case []int:
								intSlice := make([]int, len(sliceVal))
								for i, item := range sliceVal {
									if intVal, ok := item.(float64); ok {
										intSlice[i] = int(intVal)
									} else {
										Logger.Warnf("Failed to convert %v to int for %s[%d]", item, fullKey, i)
									}
								}
								c.configData[fullKey] = intSlice
								continue
							case []bool:
								boolSlice := make([]bool, len(sliceVal))
								for i, item := range sliceVal {
									if boolVal, ok := item.(bool); ok {
										boolSlice[i] = boolVal
									} else {
										Logger.Warnf("Failed to convert %v to bool for %s[%d]", item, fullKey, i)
									}
								}
								c.configData[fullKey] = boolSlice
								continue
							case []float32:
								floatSlice := make([]float32, len(sliceVal))
								for i, item := range sliceVal {
									if floatVal, ok := item.(float64); ok {
										floatSlice[i] = float32(floatVal)
									} else {
										Logger.Warnf("Failed to convert %v to float32 for %s[%d]", item, fullKey, i)
									}
								}
								c.configData[fullKey] = floatSlice
								continue
							case []float64:
								floatSlice := make([]float64, len(sliceVal))
								for i, item := range sliceVal {
									if floatVal, ok := item.(float64); ok {
										floatSlice[i] = floatVal
									} else {
										Logger.Warnf("Failed to convert %v to float64 for %s[%d]", item, fullKey, i)
									}
								}
								c.configData[fullKey] = floatSlice
								continue
							}
						}
					}

					// Handle map conversions
					if reflect.TypeOf(existingVal) != nil && reflect.TypeOf(existingVal).Kind() == reflect.Map {
						if mapVal, ok := v.(map[string]interface{}); ok {
							switch existingVal.(type) {
							case map[string]string:
								strMap := make(map[string]string)
								for k, item := range mapVal {
									if item != nil {
										strMap[k] = fmt.Sprintf("%v", item)
									}
								}
								c.configData[fullKey] = strMap
								continue
							case map[string]interface{}:
								c.configData[fullKey] = mapVal
								continue
							}
						}
					}
				}

				// Check if this field should be ignored (has hyphen tag)
				if c.shouldIgnoreField(fullKey) {
					continue
				}

				// Try to set the value with proper type conversion
				if err := c.set(fullKey, v); err != nil {
					// Log type conversion errors but don't fail the entire config load
					Logger.Warnf("Error setting config key %s: %v", fullKey, err)
				}
			}
		}
	}

	processMap(rawConfig, "")
	return nil
}

func (c *Structure) setupConfigSaver() {
	// Only set up config saver if explicitly enabled
	if c.autoSave {
		configsToSave = append(configsToSave, c)

		once.Do(func() {
			signalChannel = make(chan os.Signal, 1)
			signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
			go func() {
				defer func() {
					// Clean up signal channel on exit
					signal.Stop(signalChannel)
					close(signalChannel)
				}()

				<-signalChannel
				for _, config := range configsToSave {
					if config.changed {
						Logger.Info("Saving config before exit...")
						if err := config.saveConfig(); err != nil {
							Logger.Errorf("Error saving configuration: %v", err)
						}
					}
				}
				os.Exit(0)
			}()
		})
	}
}

// CleanupSignalHandler allows for graceful cleanup of the signal handler
// This should be called in tests or when the application wants to clean up
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
		// Logger.Infof("GetHelpTag InitSelf %s", c.name)
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

// shouldIgnoreField checks if a field should be ignored based on its tag
func (c *Structure) shouldIgnoreField(key string) bool {
	v := reflect.ValueOf(c.parent)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}

	t := v.Type()

	// Check all fields in the struct
	var checkStruct func(reflect.Type, string) bool
	checkStruct = func(t reflect.Type, prefix string) bool {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Get the config name for this field
			configVarName := c.getConfigNameFromField(field)

			// If this field has a hyphen tag, it should be ignored
			if configVarName == "-" {
				fieldName := field.Name
				if prefix != "" {
					fieldName = prefix + "." + fieldName
				}

				// Check if the key matches this field name
				if key == fieldName {
					return true
				}
			}

			// Check nested structs
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

	return checkStruct(t, "")
}

func (c *Structure) saveConfig() error {
	if c.configHandler == nil {
		return nil
	}

	data, err := json.Marshal(c.configData)
	if err != nil {
		return c.WrapError(err, 0, "")
	}

	if err := c.configHandler.SaveConfig(data); err != nil {
		return c.WrapError(err, 0, "")
	}

	return nil
}

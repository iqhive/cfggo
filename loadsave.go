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

func (c *Structure) loadConfig() error {
	if c.configHandler == nil {
		return ErrorWrapper(nil, 400, "configSource is nil")
	}

	data, err := c.configHandler.LoadConfig()
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}

	// if c.configHandler.SaveConfig != nil {
	// 	// Logger.Debug("Setting up config saver")
	// }
	c.setupConfigSaver()

	return c.loadJSONConfigFromBytes(data)
}

func (c *Structure) loadJSONConfigFromBytes(data []byte) error {
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return ErrorWrapper(err, 0, "")
	}

	configMutex.Lock()
	defer configMutex.Unlock()

	var processMap func(map[string]interface{}, string) error
	processMap = func(m map[string]interface{}, prefix string) error {
		for key, value := range m {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}

			switch v := value.(type) {
			case map[string]interface{}:
				// Process nested maps
				if err := processMap(v, fullKey); err != nil {
					return err
				}
				
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
							}
						}
					}
					
					// Handle time.Duration conversion
					if _, isDuration := existingVal.(time.Duration); isDuration {
						if strVal, ok := v.(string); ok {
							if duration, err := time.ParseDuration(strVal); err == nil {
								c.configData[fullKey] = duration
								continue
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
									}
								}
								c.configData[fullKey] = intSlice
								continue
							case []bool:
								boolSlice := make([]bool, len(sliceVal))
								for i, item := range sliceVal {
									if boolVal, ok := item.(bool); ok {
										boolSlice[i] = boolVal
									}
								}
								c.configData[fullKey] = boolSlice
								continue
							case []float32:
								floatSlice := make([]float32, len(sliceVal))
								for i, item := range sliceVal {
									if floatVal, ok := item.(float64); ok {
										floatSlice[i] = float32(floatVal)
									}
								}
								c.configData[fullKey] = floatSlice
								continue
							case []float64:
								floatSlice := make([]float64, len(sliceVal))
								for i, item := range sliceVal {
									if floatVal, ok := item.(float64); ok {
										floatSlice[i] = floatVal
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
				
				// Try to set the value with proper type conversion
				if err := c.set(fullKey, v); err != nil {
					Logger.Warn("Error setting config key %s: %v", fullKey, err)
				}
			}
		}
		return nil
	}

	return processMap(rawConfig, "")
}

func (c *Structure) setupConfigSaver() {
	// Only set up config saver if explicitly enabled
	if c.autoSave {
		configsToSave = append(configsToSave, c)

		once.Do(func() {
			sigchan := make(chan os.Signal, 1)
			signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigchan
				for _, config := range configsToSave {
					if config.changed {
						Logger.Info("Saving config before exit...")
						if err := config.saveConfig(); err != nil {
							Logger.Error("Error saving configuration: %v", err)
						}
					}
				}
				os.Exit(0)
			}()
		})
	}
}

func (c *Structure) GetJSONBytes() []byte {
	data, _ := json.Marshal(c.configData)
	return data
}

func (c *Structure) String() string {
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

func (c *Structure) saveConfig() error {
	if c.configHandler == nil {
		return nil
	}

	data, err := json.Marshal(c.configData)
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}

	if err := c.configHandler.SaveConfig(data); err != nil {
		return ErrorWrapper(err, 0, "")
	}

	return nil
}

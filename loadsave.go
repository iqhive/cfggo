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
var once sync.Once

func (c *Structure) loadConfig() error {
	if c.configHandler == nil {
		return ErrorWrapper(nil, 400, "configSource is nil")
	}

	data, err := c.configHandler.LoadConfig()
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}

	if c.configHandler.SaveConfig != nil {
		Logger.Debug("Setting up config saver")
		c.setupConfigSaver()
	}

	return c.loadJSONConfigFromBytes(data)
}

func (c *Structure) loadJSONConfigFromBytes(data []byte) error {
	tempConfigData := c.createStruct()
	if err := json.Unmarshal(data, tempConfigData); err != nil {
		return ErrorWrapper(err, 0, "")
	}

	rvalue := reflect.ValueOf(tempConfigData).Elem()
	rtype := rvalue.Type()

	configMutex.Lock()
	defer configMutex.Unlock()
	for i := 0; i < rvalue.NumField(); i++ {
		field := rtype.Field(i)
		configKey := c.getConfigNameFromField(field)
		if configKey == "" || configKey == "-" {
			continue
		}
		err := c.set(configKey, rvalue.Field(i).Interface())
		if err != nil {
			Logger.Warn("loadConfig error setting %s to (%v): %v", configKey, rvalue.Field(i).Interface(), err)
		}
	}

	return nil
}

func (c *Structure) setupConfigSaver() {
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

func (c *Structure) GetJSONBytes() []byte {
	data, _ := json.Marshal(c.configData)
	return data
}

func (c *Structure) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s:\n", c.name))
	maxKeyLen := 0
	for key := range c.configData {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}
	}
	for key, value := range c.configData {
		sb.WriteString(fmt.Sprintf("%*s: %v\n", maxKeyLen, key, value))
	}
	return sb.String()
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

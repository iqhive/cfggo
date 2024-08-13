package cfggo

import (
	"encoding/json"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"bitbucket.org/iqhive/apierror/v3"
	"bitbucket.org/iqhive/iqlog/v3"
)

var configsToSave []*GenericConfig
var once sync.Once

func (c *GenericConfig) SetupConfigSaver() {
	configsToSave = append(configsToSave, c)

	once.Do(func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigchan
			for _, config := range configsToSave {
				if config.changed {
					iqlog.Infof("Saving %s before exit...", config.filename)
					if err := config.saveConfig(); err != nil {
						iqlog.Errorf("Error saving configuration: %v", err)
					}
				}
			}
			os.Exit(0)
		}()
	})
}

func (c *GenericConfig) saveConfig() error {
	file, err := os.Create(c.filename)
	if err != nil {
		return apierror.New(err, 0, "")
	}
	defer file.Close()

	tempConfigData := make(map[string]interface{})
	for key, value := range c.configData {
		if reflect.TypeOf(value) == reflect.TypeOf(time.Time{}) {
			tempConfigData[key] = value.(time.Time).Format(time.RFC3339)
		} else if reflect.TypeOf(value) == reflect.TypeOf(time.Duration(0)) {
			tempConfigData[key] = value.(time.Duration).String()
		} else {
			tempConfigData[key] = value
		}
	}

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(tempConfigData); err != nil {
		return apierror.New(err, 0, "")
	}

	return nil
}

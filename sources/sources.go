package sources

import "encoding/json"

// ConfigHandler interface defines the contract for configuration sources
type ConfigHandler interface {
	// IsDefault should return true if the application may continue to start up if an error
	// is returned from [ConfigHandler.LoadConfig].
	IsDefault() bool
	// LoadConfig should load the JSON representation of the config.
	LoadConfig() (json.RawMessage, error)
	// SaveConfig should save the JSON representation of the config.
	SaveConfig(json.RawMessage) error
}

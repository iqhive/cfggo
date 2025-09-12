package sources

import (
	"encoding/json"
	"fmt"
	"os"
)

// HandlerFile implements ConfigHandler for file-based configuration
type HandlerFile struct {
	Filename      string
	defaultConfig bool
}

// NewHandlerFile creates a new file-based configuration handler
func NewHandlerFile(filename string, defaultConfig bool) *HandlerFile {
	return &HandlerFile{
		Filename:      filename,
		defaultConfig: defaultConfig,
	}
}

// IsDefault returns true if this is a default configuration handler
func (h *HandlerFile) IsDefault() bool {
	return h.defaultConfig
}

// LoadConfig loads configuration from the file
func (h *HandlerFile) LoadConfig() (json.RawMessage, error) {
	if h.Filename == "" {
		return nil, nil
	}
	data, err := os.ReadFile(h.Filename)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveConfig saves configuration to the file
func (h *HandlerFile) SaveConfig(data json.RawMessage) error {
	if h.Filename == "" {
		return fmt.Errorf("filename is empty")
	}
	err := os.WriteFile(h.Filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

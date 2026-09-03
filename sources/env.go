package sources

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// HandlerEnv implements ConfigHandler for environment variable-based configuration
type HandlerEnv struct {
	prefix        string
	defaultConfig bool
}

// NewHandlerEnv creates a new environment variable-based configuration handler
func NewHandlerEnv(prefix string, defaultConfig bool) *HandlerEnv {
	return &HandlerEnv{
		prefix:        prefix,
		defaultConfig: defaultConfig,
	}
}

// IsDefault returns true if this is a default configuration handler
func (h *HandlerEnv) IsDefault() bool {
	return h.defaultConfig
}

// LoadConfig loads configuration from environment variables
func (h *HandlerEnv) LoadConfig() (json.RawMessage, error) {
	// An empty prefix would import the entire process environment into the
	// config map — including unrelated variables that may hold other
	// processes' secrets. Require an explicit prefix so only variables
	// intended for this application are loaded. (cfggo's built-in env override
	// layer, which matches variables to known struct keys, does not use this
	// handler and is unaffected.)
	if h.prefix == "" {
		return nil, errors.New("HandlerEnv requires a non-empty prefix: an empty prefix would load the entire process environment into the configuration")
	}

	config := make(map[string]interface{})

	// Get all environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Apply prefix filter if specified, stripping the prefix from the raw
		// environment variable name BEFORE converting "_" to "."
		// Stripping after conversion fails because the prefix's own underscores
		// would already have become dots (e.g. prefix "MYAPP_" never matches the
		// converted "myapp.port"), leaving the prefix stuck on the key
		if h.prefix != "" {
			if !strings.HasPrefix(key, h.prefix) {
				continue
			}
			key = strings.TrimPrefix(key, h.prefix)
		}

		// Convert environment variable name to config key
		configKey := strings.ToLower(strings.ReplaceAll(key, "_", "."))
		// A prefix without a trailing separator (e.g. "MYAPP") leaves a leading
		// "_" that becomes a leading "."; drop it so keys line up with config
		configKey = strings.TrimPrefix(configKey, ".")

		// Try to parse as JSON first, fallback to string. Numbers are kept as
		// json.Number so the literal is re-emitted verbatim by json.Marshal and
		// a 64-bit integer is not rounded through float64 on the way
		var parsedValue interface{}
		dec := json.NewDecoder(strings.NewReader(value))
		dec.UseNumber()
		if err := dec.Decode(&parsedValue); err != nil || dec.More() {
			parsedValue = value
		}

		config[configKey] = parsedValue
	}

	return json.Marshal(config)
}

// SaveConfig saves configuration to environment variables
// Note: This is a no-op as environment variables cannot be modified by the process
func (h *HandlerEnv) SaveConfig(data json.RawMessage) error {
	// Environment variables cannot be modified by the process
	// This is a read-only configuration source
	return nil
}

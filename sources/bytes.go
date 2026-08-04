package sources

import "encoding/json"

// HandlerBytes is an in-memory ConfigHandler backed by a JSON blob. It is used
// by cfggo.Group to inject a member's section of a combined configuration file
// into the member's existing file-loading path without duplicating the JSON
// parsing and type-coercion logic.
type HandlerBytes struct {
	Data json.RawMessage
}

// NewHandlerBytes creates a new in-memory configuration handler that returns
// the provided JSON bytes from LoadConfig.
func NewHandlerBytes(data json.RawMessage) *HandlerBytes {
	return &HandlerBytes{Data: data}
}

// IsDefault returns true so a missing or empty in-memory section is treated as
// an optional source rather than a fatal load error.
func (h *HandlerBytes) IsDefault() bool { return true }

// LoadConfig returns the in-memory JSON configuration.
func (h *HandlerBytes) LoadConfig() (json.RawMessage, error) { return h.Data, nil }

// SaveConfig stores the JSON configuration in memory.
func (h *HandlerBytes) SaveConfig(data json.RawMessage) error {
	h.Data = data
	return nil
}

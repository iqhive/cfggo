package sources

import (
	"encoding/json"
	"sync"
)

// HandlerBytes implements ConfigHandler for an in-memory JSON document.
type HandlerBytes struct {
	mu            sync.RWMutex
	data          []byte
	defaultConfig bool
}

// NewHandlerBytes creates an in-memory configuration handler.
func NewHandlerBytes(data []byte, defaultConfig bool) *HandlerBytes {
	h := &HandlerBytes{defaultConfig: defaultConfig}
	h.Replace(data)
	return h
}

// IsDefault returns true if this is a default configuration handler.
func (h *HandlerBytes) IsDefault() bool { return h.defaultConfig }

// LoadConfig returns the current document.
func (h *HandlerBytes) LoadConfig() (json.RawMessage, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append(json.RawMessage(nil), h.data...), nil
}

// SaveConfig replaces the current document.
func (h *HandlerBytes) SaveConfig(data json.RawMessage) error {
	h.Replace(data)
	return nil
}

// Replace replaces the current document.
func (h *HandlerBytes) Replace(data []byte) {
	h.mu.Lock()
	h.data = append(h.data[:0], data...)
	h.mu.Unlock()
}

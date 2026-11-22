package sources

import (
	"encoding/json"
	"sync"
)

// HandlerBytes implements ConfigHandler over an in-memory JSON document. It
// lets a caller feed configuration through the exact same load path as a file
// (type coercion, provenance, strict/lenient handling) without touching disk.
//
// Group uses it to hand each member service its slice of a combined
// configuration file. It is safe for concurrent use.
type HandlerBytes struct {
	mu            sync.RWMutex
	data          json.RawMessage
	saved         json.RawMessage
	defaultConfig bool
	onSave        func() error
}

// NewHandlerBytes creates an in-memory configuration handler over a copy of
// data. When defaultConfig is true a load failure is not fatal to Init.
func NewHandlerBytes(data json.RawMessage, defaultConfig bool) *HandlerBytes {
	return &HandlerBytes{data: cloneRawMessage(data), defaultConfig: defaultConfig}
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	if data == nil {
		return nil
	}
	cloned := make(json.RawMessage, len(data))
	copy(cloned, data)
	return cloned
}

// IsDefault returns true if this is a default configuration handler.
func (h *HandlerBytes) IsDefault() bool { return h.defaultConfig }

// LoadConfig returns a copy of the in-memory document.
func (h *HandlerBytes) LoadConfig() (json.RawMessage, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneRawMessage(h.data), nil
}

// SaveConfig records a copy of the document (retrieve it with Saved) and then
// runs the OnSave callback, if any, so the owner of the handler can persist it.
// Without a callback the document is only recorded.
func (h *HandlerBytes) SaveConfig(data json.RawMessage) error {
	h.mu.Lock()
	h.saved = cloneRawMessage(data)
	onSave := h.onSave
	h.mu.Unlock()
	if onSave != nil {
		return onSave()
	}
	return nil
}

// OnSave registers fn to run after SaveConfig has recorded a document. Group
// uses it so that a member's own Save writes the combined configuration file
// instead of silently recording the document in memory.
func (h *HandlerBytes) OnSave(fn func() error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onSave = fn
}

// SetData replaces the document returned by LoadConfig with a copy of data, so
// a subsequent Reload sees fresh values.
func (h *HandlerBytes) SetData(data json.RawMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = cloneRawMessage(data)
}

// Saved returns a copy of the most recent document passed to SaveConfig, or nil.
func (h *HandlerBytes) Saved() json.RawMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneRawMessage(h.saved)
}

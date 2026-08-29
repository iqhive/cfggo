package sources

import (
	"encoding/json"
	"testing"
)

func TestHandlerBytesDefaultAndLoad(t *testing.T) {
	h := NewHandlerBytes(json.RawMessage(`{"port":8080}`), true)
	if !h.IsDefault() {
		t.Fatal("IsDefault() = false, want true")
	}

	data, err := h.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if string(data) != `{"port":8080}` {
		t.Fatalf("LoadConfig() = %s, want the original document", string(data))
	}
}

func TestHandlerBytesNotDefault(t *testing.T) {
	h := NewHandlerBytes(json.RawMessage(`{}`), false)
	if h.IsDefault() {
		t.Fatal("IsDefault() = true, want false")
	}
}

func TestHandlerBytesSaveAndSaved(t *testing.T) {
	h := NewHandlerBytes(json.RawMessage(`{"port":8080}`), false)

	if got := h.Saved(); got != nil {
		t.Fatalf("Saved() before any save = %s, want nil", string(got))
	}

	if err := h.SaveConfig(json.RawMessage(`{"port":9090}`)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	saved := h.Saved()
	if string(saved) != `{"port":9090}` {
		t.Fatalf("Saved() = %s, want the saved document", string(saved))
	}

	// Saving must not mutate the document returned by LoadConfig.
	data, err := h.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if string(data) != `{"port":8080}` {
		t.Fatalf("LoadConfig() after save = %s, want the original document", string(data))
	}
}

func TestHandlerBytesSetData(t *testing.T) {
	h := NewHandlerBytes(json.RawMessage(`{"port":8080}`), false)

	h.SetData(json.RawMessage(`{"port":7777}`))

	data, err := h.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if string(data) != `{"port":7777}` {
		t.Fatalf("LoadConfig() after SetData = %s, want the new document", string(data))
	}
}

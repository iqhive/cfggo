package sources

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestHandlerBytesOnSaveRunsAfterRecording(t *testing.T) {
	h := NewHandlerBytes(json.RawMessage(`{}`), false)
	calls := 0
	h.OnSave(func() error {
		calls++
		if string(h.Saved()) != `{"port":1}` {
			t.Errorf("Saved() inside OnSave = %s, want the document already recorded", h.Saved())
		}
		return nil
	})
	if err := h.SaveConfig(json.RawMessage(`{"port":1}`)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if calls != 1 {
		t.Fatalf("OnSave calls = %d, want 1", calls)
	}

	boom := errors.New("disk full")
	h.OnSave(func() error { return boom })
	if err := h.SaveConfig(json.RawMessage(`{"port":2}`)); !errors.Is(err, boom) {
		t.Fatalf("SaveConfig error = %v, want the OnSave error propagated", err)
	}
}

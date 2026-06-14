package sources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerHTTPSupportsLoadOnlyAndSaveOnly(t *testing.T) {
	t.Run("save only skips load", func(t *testing.T) {
		dest := httptest.NewRequest(http.MethodPost, "http://example.test/config", nil)
		handler := NewHandlerHTTP(nil, dest, false)

		data, err := handler.LoadConfig()
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if data != nil {
			t.Fatalf("LoadConfig() data = %s, want nil", string(data))
		}
	})

	t.Run("load only skips save", func(t *testing.T) {
		source := httptest.NewRequest(http.MethodGet, "http://example.test/config", nil)
		handler := NewHandlerHTTP(source, nil, false)

		if err := handler.SaveConfig(json.RawMessage(`{"ok":true}`)); err != nil {
			t.Fatalf("SaveConfig() error = %v", err)
		}
	})
}

func TestHandlerHTTPNilURLsReturnErrors(t *testing.T) {
	if _, err := NewHandlerHTTP(&http.Request{}, nil, false).LoadConfig(); err == nil {
		t.Fatal("LoadConfig() with nil URL: expected error, got nil")
	}
	if err := NewHandlerHTTP(nil, &http.Request{}, false).SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig() with nil URL: expected error, got nil")
	}
}

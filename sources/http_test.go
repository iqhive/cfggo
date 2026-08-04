package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func TestHandlerHTTPLoadAndSaveRoundTripRequests(t *testing.T) {
	var savedBody string
	var savedContentType string
	var savedHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/load":
			if got := r.Header.Get("X-Load"); got != "yes" {
				t.Errorf("load header X-Load = %q, want yes", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"loaded":true}`)
		case "/save":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll(save body) error = %v", err)
			}
			savedBody = string(body)
			savedContentType = r.Header.Get("Content-Type")
			savedHeader = r.Header.Get("X-Save")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := httptest.NewRequest(http.MethodGet, server.URL+"/load", nil)
	source.Header.Set("X-Load", "yes")
	dest := httptest.NewRequest(http.MethodPut, server.URL+"/save", nil)
	dest.Header.Set("X-Save", "yes")
	handler := NewHandlerHTTP(source, dest, true)

	if !handler.IsDefault() {
		t.Fatal("IsDefault() = false, want true")
	}

	data, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if string(data) != `{"loaded":true}` {
		t.Fatalf("LoadConfig() = %s, want loaded JSON", string(data))
	}

	if err := handler.SaveConfig(json.RawMessage(`{"saved":true}`)); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if savedBody != `{"saved":true}` {
		t.Fatalf("saved body = %q, want saved JSON", savedBody)
	}
	if savedContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", savedContentType)
	}
	if savedHeader != "yes" {
		t.Fatalf("X-Save = %q, want yes", savedHeader)
	}
}

func TestHandlerHTTPStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()

	source := httptest.NewRequest(http.MethodGet, server.URL, nil)
	if _, err := NewHandlerHTTP(source, nil, false).LoadConfig(); err == nil {
		t.Fatal("LoadConfig() with non-200 response: expected error, got nil")
	}

	dest := httptest.NewRequest(http.MethodPost, server.URL, bytes.NewReader(nil))
	if err := NewHandlerHTTP(nil, dest, false).SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig() with non-200 response: expected error, got nil")
	}
}

func TestHandlerHTTPLoadReplaysRequestBody(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(load body) error = %v", err)
		}
		bodies = append(bodies, string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	source := httptest.NewRequest(http.MethodPost, server.URL, bytes.NewBufferString(`{"query":"cfg"}`))
	handler := NewHandlerHTTP(source, nil, false)
	for i := 0; i < 2; i++ {
		if _, err := handler.LoadConfig(); err != nil {
			t.Fatalf("LoadConfig(%d) error = %v", i, err)
		}
	}
	if len(bodies) != 2 || bodies[0] != `{"query":"cfg"}` || bodies[1] != `{"query":"cfg"}` {
		t.Fatalf("load bodies = %#v, want replayed body twice", bodies)
	}
}

func TestHandlerHTTPPreservesRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not receive request after context cancellation")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	source := httptest.NewRequest(http.MethodGet, server.URL, nil).WithContext(ctx)
	if _, err := NewHandlerHTTP(source, nil, false).LoadConfig(); err == nil {
		t.Fatal("LoadConfig() with canceled context: expected error, got nil")
	}

	dest := httptest.NewRequest(http.MethodPost, server.URL, nil).WithContext(ctx)
	if err := NewHandlerHTTP(nil, dest, false).SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig() with canceled context: expected error, got nil")
	}
}

func TestHandlerHTTPRefusesPlaintextNonLoopback(t *testing.T) {
	source := httptest.NewRequest(http.MethodGet, "http://example.test/config", nil)
	if _, err := NewHandlerHTTP(source, nil, false).LoadConfig(); err == nil {
		t.Fatal("LoadConfig() over plaintext http to non-loopback host: expected error, got nil")
	}

	dest := httptest.NewRequest(http.MethodPost, "http://example.test/config", nil)
	if err := NewHandlerHTTP(nil, dest, false).SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig() over plaintext http to non-loopback host: expected error, got nil")
	}

	// Opting in via AllowInsecureHTTP passes the scheme check (the request
	// itself will fail to connect, which is fine for this test).
	h := NewHandlerHTTP(source, nil, false)
	h.AllowInsecureHTTP = true
	if err := h.checkURLScheme(h.source.URL); err != nil {
		t.Fatalf("checkURLScheme with AllowInsecureHTTP: %v", err)
	}
}

func TestHandlerHTTPAllowsLoopbackHTTP(t *testing.T) {
	h := &HandlerHTTP{}
	for _, u := range []string{"http://127.0.0.1:8080/c", "http://localhost/c", "http://[::1]/c"} {
		req := httptest.NewRequest(http.MethodGet, u, nil)
		if err := h.checkURLScheme(req.URL); err != nil {
			t.Fatalf("checkURLScheme(%s): %v", u, err)
		}
	}
}

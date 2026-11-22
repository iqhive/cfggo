package sources

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHTTPCustomClientStillRefusesRedirectDowngrade(t *testing.T) {
	for _, operation := range []string{"load", "save"} {
		t.Run(operation, func(t *testing.T) {
			var requests atomic.Int32
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests.Add(1)
				if req.URL.Hostname() == "config.example.test" {
					return &http.Response{
						StatusCode: http.StatusFound,
						Header:     http.Header{"Location": []string{"http://plaintext.example.test/config"}},
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			})}

			source, _ := http.NewRequest(http.MethodGet, "https://config.example.test/config", nil)
			dest, _ := http.NewRequest(http.MethodPost, "https://config.example.test/config", nil)
			handler := NewHandlerHTTP(source, dest, false)
			handler.Client = client

			var err error
			if operation == "load" {
				_, err = handler.LoadConfig()
			} else {
				err = handler.SaveConfig(json.RawMessage(`{"secret":"value"}`))
			}
			if err == nil || !strings.Contains(err.Error(), "plaintext http URL") {
				t.Fatalf("%s redirect error = %v, want plaintext downgrade refusal", operation, err)
			}
			if got := requests.Load(); got != 1 {
				t.Fatalf("%s made %d requests, want the redirect target never contacted", operation, got)
			}
		})
	}
}

func TestHTTPRequestBodyIsBoundedBeforeLoad(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	source := httptest.NewRequest(http.MethodPost, server.URL, bytes.NewReader(bytes.Repeat([]byte{' '}, maxHTTPResponseBytes+1)))
	handler := NewHandlerHTTP(source, nil, false)
	if _, err := handler.LoadConfig(); err == nil || !strings.Contains(err.Error(), "request body") {
		t.Fatalf("LoadConfig with oversized request body error = %v, want a request-body limit error", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("oversized request reached the server %d times", got)
	}
}

func TestHandlerBytesDoesNotAliasCallerBuffers(t *testing.T) {
	t.Run("constructor", func(t *testing.T) {
		input := json.RawMessage(`{"port":1}`)
		handler := NewHandlerBytes(input, false)
		input[len(input)-2] = '9'
		assertJSONDocument(t, handler.LoadConfig, `{"port":1}`)
	})

	t.Run("load result", func(t *testing.T) {
		handler := NewHandlerBytes(json.RawMessage(`{"port":1}`), false)
		loaded, err := handler.LoadConfig()
		if err != nil {
			t.Fatal(err)
		}
		loaded[len(loaded)-2] = '9'
		assertJSONDocument(t, handler.LoadConfig, `{"port":1}`)
	})

	t.Run("set data", func(t *testing.T) {
		handler := NewHandlerBytes(nil, false)
		input := json.RawMessage(`{"port":1}`)
		handler.SetData(input)
		input[len(input)-2] = '9'
		assertJSONDocument(t, handler.LoadConfig, `{"port":1}`)
	})

	t.Run("save input and result", func(t *testing.T) {
		handler := NewHandlerBytes(nil, false)
		input := json.RawMessage(`{"port":1}`)
		if err := handler.SaveConfig(input); err != nil {
			t.Fatal(err)
		}
		input[len(input)-2] = '9'
		assertJSONDocument(t, func() (json.RawMessage, error) { return handler.Saved(), nil }, `{"port":1}`)

		saved := handler.Saved()
		saved[len(saved)-2] = '8'
		assertJSONDocument(t, func() (json.RawMessage, error) { return handler.Saved(), nil }, `{"port":1}`)
	})
}

func assertJSONDocument(t *testing.T, load func() (json.RawMessage, error), want string) {
	t.Helper()
	got, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("document = %s, want %s", got, want)
	}
}

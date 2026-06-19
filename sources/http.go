package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HandlerHTTP implements ConfigHandler for HTTP-based configuration
type HandlerHTTP struct {
	source        *http.Request
	dest          *http.Request
	defaultConfig bool
	sourceBody    []byte
	sourceBodyErr error
}

// NewHandlerHTTP creates a new HTTP-based configuration handler
func NewHandlerHTTP(source, dest *http.Request, defaultConfig bool) *HandlerHTTP {
	handler := &HandlerHTTP{
		defaultConfig: defaultConfig,
	}
	if source != nil {
		handler.source = source.Clone(source.Context())
		handler.sourceBody, handler.sourceBodyErr = snapshotRequestBody(source)
	}
	if dest != nil {
		handler.dest = dest.Clone(dest.Context())
	}
	return handler
}

func snapshotRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		defer body.Close()
		return io.ReadAll(body)
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

// IsDefault returns true if this is a default configuration handler
func (h *HandlerHTTP) IsDefault() bool {
	return h.defaultConfig
}

// LoadConfig loads configuration from the HTTP source
func (h *HandlerHTTP) LoadConfig() (json.RawMessage, error) {
	if h.source == nil {
		return nil, nil
	}
	if h.source.URL == nil || h.source.URL.String() == "" {
		return nil, fmt.Errorf("source URL is empty")
	}
	if h.sourceBodyErr != nil {
		return nil, h.sourceBodyErr
	}

	var body io.Reader
	if h.sourceBody != nil {
		body = bytes.NewReader(h.sourceBody)
	}
	req, err := http.NewRequestWithContext(h.source.Context(), h.source.Method, h.source.URL.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header = h.source.Header.Clone()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to load config from HTTP source: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveConfig saves configuration to the HTTP destination
func (h *HandlerHTTP) SaveConfig(data json.RawMessage) error {
	if h.dest == nil {
		return nil
	}
	if h.dest.URL == nil || h.dest.URL.String() == "" {
		return fmt.Errorf("destination URL is empty")
	}

	req, err := http.NewRequestWithContext(h.dest.Context(), h.dest.Method, h.dest.URL.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header = h.dest.Header.Clone()
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to save config to HTTP destination: status %d", resp.StatusCode)
	}
	return nil
}

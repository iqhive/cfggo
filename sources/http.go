package sources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// maxHTTPResponseBytes bounds the size of a configuration payload read from an
// HTTP source so a misbehaving endpoint cannot exhaust process memory.
const maxHTTPResponseBytes = 10 << 20 // 10 MiB

// defaultHTTPClient is used when no custom Client is supplied. Unlike
// http.DefaultClient it carries a timeout so a hung config endpoint cannot
// block Init or Reload indefinitely.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// HandlerHTTP implements ConfigHandler for HTTP-based configuration
type HandlerHTTP struct {
	source        *http.Request
	dest          *http.Request
	defaultConfig bool
	sourceBody    []byte
	sourceBodyErr error

	// AllowInsecureHTTP permits plain-HTTP (non-TLS) URLs for non-loopback
	// hosts. By default only https:// URLs and http:// URLs pointing at a
	// loopback address (localhost, 127.0.0.0/8, ::1) are accepted, so
	// configuration (which may include secrets on save) never transits a
	// network unencrypted unless explicitly opted into.
	AllowInsecureHTTP bool

	// Client is the HTTP client used for requests. When nil, a shared default
	// client with a 30s timeout is used.
	Client *http.Client
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

func (h *HandlerHTTP) httpClient() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return defaultHTTPClient
}

// checkURLScheme rejects plaintext HTTP URLs for non-loopback hosts unless
// AllowInsecureHTTP is set.
func (h *HandlerHTTP) checkURLScheme(u *url.URL) error {
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if h.AllowInsecureHTTP || isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("plaintext http URL %q refused for non-loopback host: use https, or set AllowInsecureHTTP to opt in", u.String())
	default:
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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
	if err := h.checkURLScheme(h.source.URL); err != nil {
		return nil, err
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

	resp, err := h.httpClient().Do(req)
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

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHTTPResponseBytes {
		return nil, fmt.Errorf("config response from HTTP source exceeds %d bytes", maxHTTPResponseBytes)
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
	if err := h.checkURLScheme(h.dest.URL); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(h.dest.Context(), h.dest.Method, h.dest.URL.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header = h.dest.Header.Clone()
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient().Do(req)
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

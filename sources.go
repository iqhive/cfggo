package cfggo

import (
	"bytes"
	"io"
	"net/http"
	"os"
)

type configHandler interface {
	LoadConfig() ([]byte, error)
	SaveConfig(data []byte) error
}

type handlerFile struct {
	filename string
}

func (h *handlerFile) LoadConfig() ([]byte, error) {
	if h.filename == "" {
		return nil, nil
	}
	data, err := os.ReadFile(h.filename)
	if err != nil {
		return nil, ErrorWrapper(err, 0, "")
	}
	return data, nil
}

func (h *handlerFile) SaveConfig(data []byte) error {
	if h.filename == "" {
		return ErrorWrapper(nil, 400, "filename is empty")
	}
	err := os.WriteFile(h.filename, data, 0644)
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}
	return nil
}

type handlerHTTP struct {
	source http.Request
	dest   http.Request
}

func (h *handlerHTTP) LoadConfig() ([]byte, error) {
	if h.source.URL.String() == "" {
		return nil, ErrorWrapper(nil, 400, "source URL is empty")
	}

	req, err := http.NewRequest(h.source.Method, h.source.URL.String(), h.source.Body)
	if err != nil {
		return nil, ErrorWrapper(err, 0, "")
	}
	req.Header = h.source.Header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, ErrorWrapper(err, 0, "")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrorWrapper(nil, resp.StatusCode, "failed to load config from HTTP source")
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, 0, "")
	}
	return data, nil
}

func (h *handlerHTTP) SaveConfig(data []byte) error {
	if h.dest.URL.String() == "" {
		return ErrorWrapper(nil, 400, "destination URL is empty")
	}

	req, err := http.NewRequest(h.dest.Method, h.dest.URL.String(), bytes.NewReader(data))
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}
	req.ContentLength = int64(len(data))
	req.Header = h.dest.Header
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorWrapper(err, 0, "")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrorWrapper(nil, resp.StatusCode, "failed to save config to HTTP destination")
	}
	return nil
}

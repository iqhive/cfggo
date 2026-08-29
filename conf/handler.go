package conf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/iqhive/cfggo/sources"
)

type handlerSettings struct {
	codec   *Codec
	rewrite bool
}

// HandlerOption configures a FileHandler.
type HandlerOption func(*handlerSettings) error

// WithCodec uses codec for file translation.
func WithCodec(codec *Codec) HandlerOption {
	return func(settings *handlerSettings) error {
		if codec == nil {
			return fmt.Errorf("conf: codec must not be nil")
		}
		settings.codec = codec
		return nil
	}
}

// RewriteOnSave permits SaveConfig to replace the file with deterministic conf
// output. Without it, handlers are read-only.
func RewriteOnSave() HandlerOption {
	return func(settings *handlerSettings) error {
		settings.rewrite = true
		return nil
	}
}

// FileHandler translates a conf file to and from cfggo's canonical JSON.
type FileHandler struct {
	filename      string
	defaultConfig bool
	codec         *Codec
	rewrite       bool
}

// NewFileHandler constructs a conf file handler. It does not require the file
// to exist until LoadConfig is called.
func NewFileHandler(filename string, defaultConfig bool, options ...HandlerOption) (*FileHandler, error) {
	codec, _ := New()
	settings := handlerSettings{codec: codec}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&settings); err != nil {
			return nil, err
		}
	}
	if filename == "" {
		return nil, fmt.Errorf("conf: filename must not be empty")
	}
	return &FileHandler{filename: filename, defaultConfig: defaultConfig, codec: settings.codec, rewrite: settings.rewrite}, nil
}

// IsDefault reports whether cfggo may continue when loading fails.
func (h *FileHandler) IsDefault() bool { return h.defaultConfig }

// LoadConfig reads and translates the conf document.
func (h *FileHandler) LoadConfig() (json.RawMessage, error) {
	file, err := os.Open(h.filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, h.codec.limits.MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	return h.codec.decode(input, h.filename)
}

// SaveConfig deterministically rewrites the conf file when RewriteOnSave was
// supplied. sources.HandlerFile is deliberately used as an atomic byte writer.
func (h *FileHandler) SaveConfig(input json.RawMessage) error {
	if !h.rewrite {
		return ErrReadOnly
	}
	output, err := h.codec.Encode(input)
	if err != nil {
		return err
	}
	return sources.NewHandlerFile(h.filename, h.defaultConfig).SaveConfig(output)
}

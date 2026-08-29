package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HandlerFile implements ConfigHandler for file-based configuration
type HandlerFile struct {
	Filename      string
	defaultConfig bool
}

// NewHandlerFile creates a new file-based configuration handler
func NewHandlerFile(filename string, defaultConfig bool) *HandlerFile {
	return &HandlerFile{
		Filename:      filename,
		defaultConfig: defaultConfig,
	}
}

// IsDefault returns true if this is a default configuration handler
func (h *HandlerFile) IsDefault() bool {
	return h.defaultConfig
}

// LoadConfig loads configuration from the file
func (h *HandlerFile) LoadConfig() (json.RawMessage, error) {
	if h.Filename == "" {
		return nil, nil
	}
	data, err := os.ReadFile(h.Filename)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveConfig saves configuration to the file
func (h *HandlerFile) SaveConfig(data json.RawMessage) error {
	if h.Filename == "" {
		return fmt.Errorf("filename is empty")
	}

	// New config files are created owner-only: saved configuration may contain
	// unmasked secrets, so it must not be world-readable. Existing files keep
	// their current permissions.
	mode := os.FileMode(0600)
	if info, err := os.Stat(h.Filename); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(h.Filename)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(h.Filename)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, h.Filename); err != nil {
		return err
	}
	cleanup = false

	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

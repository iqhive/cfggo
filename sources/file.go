package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultMaxFileBytes bounds the size of a configuration file HandlerFile will
// read, so a runaway or hostile file cannot exhaust process memory. It matches
// the cap on HTTP sources and conf documents. Raise it per handler with the
// MaxBytes field
const DefaultMaxFileBytes int64 = 10 << 20 // 10 MiB

// HandlerFile implements ConfigHandler for file-based configuration
type HandlerFile struct {
	Filename      string
	defaultConfig bool

	// MaxBytes is the largest file LoadConfig accepts; zero means
	// DefaultMaxFileBytes
	MaxBytes int64
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

// LoadConfig loads configuration from the file. A file larger than MaxBytes
// (DefaultMaxFileBytes when unset) is rejected rather than read whole
func (h *HandlerFile) LoadConfig() (json.RawMessage, error) {
	if h.Filename == "" {
		return nil, nil
	}
	limit := h.MaxBytes
	if limit <= 0 {
		limit = DefaultMaxFileBytes
	}
	file, err := os.Open(h.Filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("configuration file %s exceeds %d bytes; raise HandlerFile.MaxBytes to allow it", h.Filename, limit)
	}
	return data, nil
}

// SaveConfig saves configuration to the file
func (h *HandlerFile) SaveConfig(data json.RawMessage) error {
	if h.Filename == "" {
		return fmt.Errorf("filename is empty")
	}

	// A symbolic link is preserved and its target rewritten. Renaming the
	// temporary file over the link path would replace the link with a regular
	// file and leave the real target stale
	target, err := resolveSaveTarget(h.Filename)
	if err != nil {
		return err
	}

	// New config files are created owner-only: saved configuration may contain
	// unmasked secrets, so it must not be world-readable. Existing files keep
	// their current permissions.
	mode := os.FileMode(0600)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
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
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	cleanup = false

	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

// resolveSaveTarget returns the path SaveConfig should atomically replace.
// A regular file (or a path that does not exist yet) is returned as-is. A
// symbolic link resolves to its final target so the link survives the save;
// a dangling link resolves to the path it points at, which is then created,
// matching what an ordinary open-for-write through the link would do
func resolveSaveTarget(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return filename, nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return filename, nil
	}
	resolved, err := filepath.EvalSymlinks(filename)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	target, err := os.Readlink(filename)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(filename), target)
	}
	return target, nil
}

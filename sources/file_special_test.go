package sources

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SaveConfig must not replace a non-regular file with a regular one. The
// rename-based atomic write happily swaps a FIFO, a device, or a socket for an
// ordinary file (and /dev/null is a plausible "no persistence" config path),
// which breaks whatever depended on the special file: every later writer of a
// replaced /dev/null would fill a real file on disk.
func TestSaveConfigRefusesNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "config.json")
	if err := mkfifo(t, fifo); err != nil {
		t.Skipf("FIFOs unavailable: %v", err)
	}

	handler := NewHandlerFile(fifo, false)
	err := handler.SaveConfig(json.RawMessage(`{"port":1}`))
	if err == nil {
		t.Fatal("SaveConfig onto a FIFO: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("SaveConfig error = %q, want it to mention a non-regular file", err.Error())
	}

	info, err := os.Lstat(fifo)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO at %s was replaced by a %s; the save must leave it alone", fifo, info.Mode().Type())
	}

	// A symlink to a special file is refused the same way: the target is
	// resolved before the replace, so the guard must see the device, not the
	// link
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(fifo, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := NewHandlerFile(link, false).SaveConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("SaveConfig through a symlink to a FIFO: expected error, got nil")
	}
	info, err = os.Lstat(fifo)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("FIFO target of the symlink was replaced (mode %v, err %v)", info.Mode(), err)
	}
}

// A directory at the config path is also refused explicitly, rather than
// failing deep inside the rename with a confusing error
func TestSaveConfigRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	err := NewHandlerFile(dir, false).SaveConfig(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("SaveConfig onto a directory: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("SaveConfig error = %q, want it to mention a non-regular file", err.Error())
	}
}

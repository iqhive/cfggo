//go:build unix

package sources

import (
	"os"
	"syscall"
	"testing"
)

// mkfifo creates a named pipe at path for the save-refusal tests
func mkfifo(t *testing.T, path string) error {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return err
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return nil
}

//go:build !unix

package sources

import (
	"errors"
	"testing"
)

// mkfifo has no portable equivalent outside Unix; the refusal tests skip
func mkfifo(t *testing.T, path string) error {
	t.Helper()
	return errors.New("FIFOs are only supported on Unix")
}

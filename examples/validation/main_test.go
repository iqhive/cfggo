package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = original
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(out)
}

func TestMainExampleRuns(t *testing.T) {
	output := captureStdout(t, main)
	for _, want := range []string{
		"Configuration is valid!",
		"server_port     : 8080",
		"admin_email     : admin@example.com",
		"server_port 8080 is valid",
		"Expected error:",
		"max_connections 100 is valid",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("main output = %q, want substring %q", output, want)
		}
	}
}

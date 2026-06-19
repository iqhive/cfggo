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
	for _, key := range []string{"APP_NAME", "PORT", "DEBUG", "TIMEOUT"} {
		t.Setenv(key, "__cfggo_test_placeholder__")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%s): %v", key, err)
		}
	}

	output := captureStdout(t, main)
	for _, want := range []string{
		"=== Example 1: Basic usage",
		"App Name: my-app",
		"Port: 8080",
		"Error (with custom logger):",
		"Wrapped error:",
		"Service A - Name: service-a, Port: 9001",
		"Service B - Name: service-b, Port: 9002",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("main output = %q, want substring %q", output, want)
		}
	}
}

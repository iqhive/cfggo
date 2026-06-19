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
		"=== Blessed Startup Pattern ===",
		"load configuration:",
		"bad JSON",
		"wrong type",
		"unknown key",
		"failed validator",
		"env override    : port=9090",
		"flag override   : port=7070",
		"api_key      string  API_KEY      ****",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("main output = %q, want substring %q", output, want)
		}
	}
	if strings.Contains(output, "super-secret-token") {
		t.Fatalf("main output leaked secret value: %q", output)
	}
}

func TestInitHelpers(t *testing.T) {
	dir := t.TempDir()
	validPath := writeFile(dir, "valid.json", `{"port":9090}`)
	if err := initFromFile(validPath); err != nil {
		t.Fatalf("initFromFile(valid) error = %v", err)
	}

	unknownPath := writeFile(dir, "unknown.json", `{"port":9090,"stale_key":true}`)
	if err := initStrictFromFile(unknownPath); err == nil {
		t.Fatal("initStrictFromFile() expected unknown-key error, got nil")
	}

	invalidPath := writeFile(dir, "invalid.json", `{"admin_email":"not-an-email"}`)
	if err := initInvalidValidatedConfig(invalidPath); err == nil {
		t.Fatal("initInvalidValidatedConfig() expected validation error, got nil")
	}
}

func TestShowInitErrorSuccessPath(t *testing.T) {
	output := captureStdout(t, func() {
		showInitError("ok", nil)
	})
	if !strings.Contains(output, "unexpectedly succeeded") {
		t.Fatalf("showInitError output = %q, want success-path message", output)
	}
}

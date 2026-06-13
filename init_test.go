package cfggo

import (
	"os"
	"testing"
)

// envPrefixConfig is used to verify WithEnvConfig strips the prefix correctly.
type envPrefixConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"1"`
}

// TestWithEnvConfigStripsPrefix guards against a regression where the env
// source converted "_" to "." before stripping the prefix, so MYAPP_PORT
// produced the junk key "myapp.port" instead of mapping to "port".
func TestWithEnvConfigStripsPrefix(t *testing.T) {
	// Isolate from os.Args pollution leaked by other tests: these tests
	// auto-parse os.Args, and a stray flag would abort the whole binary
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	t.Setenv("MYAPP_PORT", "9999")

	cfg := &envPrefixConfig{}
	// WithSkipEnvironment disables the automatic env auto-loader so this test
	// exercises only the WithEnvConfig source handler.
	if err := cfg.Init(cfg, WithEnvConfig("MYAPP_"), WithSkipEnvironment()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.Port(); got != 9999 {
		t.Fatalf("Port() = %d, want 9999 (prefix not stripped correctly)", got)
	}
	if _, junk := cfg.Get("myapp.port"); junk {
		t.Fatal("unexpected junk key \"myapp.port\" present; prefix stripping is broken")
	}
}

// TestWithEnvConfigNoPrefix verifies the no-prefix path still works.
func TestWithEnvConfigNoPrefix(t *testing.T) {
	// Isolate from os.Args pollution leaked by other tests: these tests
	// auto-parse os.Args, and a stray flag would abort the whole binary
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	// Guard against leftover vars from another test affecting this one
	os.Unsetenv("MYAPP_PORT")
	t.Setenv("PORT", "4321")

	cfg := &envPrefixConfig{}
	if err := cfg.Init(cfg, WithEnvConfig(""), WithSkipEnvironment()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.Port(); got != 4321 {
		t.Fatalf("Port() = %d, want 4321", got)
	}
}

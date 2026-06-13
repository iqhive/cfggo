package cfggo

import (
	"os"
	"testing"
)

// initMyParentConfig embeds Structure to exercise InitMyParent.
type initMyParentConfig struct {
	Structure
	Host func() string `cfggo:"host" default:"localhost"`
}

// TestInitMyParentDoesNotPanic guards against a regression where InitMyParent
// dereferenced the wrong reflect field and panicked ("Elem of invalid type
// string") for every caller. It must now fail gracefully with an error.
func TestInitMyParentDoesNotPanic(t *testing.T) {
	cfg := &initMyParentConfig{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InitMyParent panicked: %v", r)
		}
	}()

	err := cfg.InitMyParent()
	if err == nil {
		t.Fatal("InitMyParent should return an actionable error, got nil")
	}
}

// envPrefixConfig is used to verify WithEnvConfig strips the prefix correctly.
type envPrefixConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"1"`
}

// TestWithEnvConfigStripsPrefix guards against a regression where the env
// source converted "_" to "." before stripping the prefix, so MYAPP_PORT
// produced the junk key "myapp.port" instead of mapping to "port".
func TestWithEnvConfigStripsPrefix(t *testing.T) {
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
	// Guard against a leftover var from another test affecting this one.
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

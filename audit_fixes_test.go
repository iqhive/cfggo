package cfggo

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"

	iflags "github.com/iqhive/cfggo/internal/flags"
)

type setValidationConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080" help:"listen port"`
}

// Set must enforce registered validators just like the flag/file/env layers,
// so a validated configuration cannot be poisoned at runtime.
func TestSetEnforcesValidators(t *testing.T) {
	cfg := &setValidationConfig{}
	if err := cfg.Init(cfg,
		WithoutFlags(),
		WithoutEnv(),
		WithValidation("port", Range(1, 65535)),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := cfg.Set("port", 99999); err == nil {
		t.Fatal("Set: expected validation error for out-of-range port, got nil")
	} else if !errors.Is(err, ErrValidation) {
		t.Fatalf("Set: error = %v, want ErrValidation", err)
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d after rejected Set, want unchanged 8080", got)
	}

	// String input must be validated as the converted type, matching storage.
	if err := cfg.Set("port", "70000"); !errors.Is(err, ErrValidation) {
		t.Fatalf("Set(string): error = %v, want ErrValidation", err)
	}
	if err := cfg.Set("port", 9090); err != nil {
		t.Fatalf("Set(valid): %v", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want 9090", got)
	}
}

// A conversion failure on a secret-tagged flag must not echo the supplied
// value: the flag package prints Set errors to stderr.
func TestSecretFlagConversionErrorRedacted(t *testing.T) {
	cv := &iflags.ConfigVar{
		Name:     "api_key_ttl",
		Want:     reflect.TypeOf(0),
		IsSecret: true,
		Setter:   func(interface{}) error { return nil },
	}
	err := cv.Set("hunter2-super-secret")
	if err == nil {
		t.Fatal("Set: expected conversion error, got nil")
	}
	if strings.Contains(err.Error(), "hunter2-super-secret") {
		t.Fatalf("secret flag conversion error leaks value: %v", err)
	}

	cv.IsSecret = false
	if err := cv.Set("not-an-int"); err == nil || !strings.Contains(err.Error(), "not-an-int") {
		t.Fatalf("non-secret flag error should keep the raw value for debuggability, got: %v", err)
	}
}

type paramNameConfig struct {
	Structure
	Name func() string `cfggo:"name" default:"svc"`
}

// WithFileConfigParamName must not panic when the config-path flag is already
// registered on the flag set (a host-owned flag on a shared set, or Init retried).
func TestFileConfigParamNameDuplicateFlagNoPanic(t *testing.T) {
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	fs.String("config", "", "host-owned config path flag")

	cfg := &paramNameConfig{}
	if err := cfg.Init(cfg,
		WithFlagSet(fs),
		WithoutEnv(),
		WithFileConfigParamName("config"),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if fs.Lookup("config") == nil {
		t.Fatal("config flag should remain registered")
	}
}

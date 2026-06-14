package cfggo

import (
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
)

type featureConfig struct {
	Structure
	Name func() string `cfggo:"name" default:"defname" help:"the name"`
	Port func() int    `cfggo:"port"`
}

func newFeatureConfig(t *testing.T) *featureConfig {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &featureConfig{Name: DefaultValue("defname"), Port: DefaultValue(0)}
	fs := flag.NewFlagSet("features", flag.ContinueOnError)
	if err := cfg.Init(cfg, WithName("features"), WithFlagSet(fs)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cfg
}

func TestProvenanceDefaultEnvAndSet(t *testing.T) {
	t.Setenv("PORT", "9090")

	cfg := newFeatureConfig(t)

	if src, ok := cfg.Source("name"); !ok || src != SourceDefault {
		t.Errorf("name source = %v (ok=%v), want default", src, ok)
	}
	if src, ok := cfg.Source("port"); !ok || src != SourceEnv {
		t.Errorf("port source = %v (ok=%v), want env", src, ok)
	}
	if cfg.Port() != 9090 {
		t.Errorf("port = %d, want 9090 (from env)", cfg.Port())
	}

	if err := cfg.Set("name", "override"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if src, _ := cfg.Source("name"); src != SourceSet {
		t.Errorf("name source after Set = %v, want set", src)
	}

	if exp := cfg.Explain(); !strings.Contains(exp, "(from set)") || !strings.Contains(exp, "(from env)") {
		t.Errorf("Explain() missing provenance annotations:\n%s", exp)
	}
}

func TestOnChangeFiresOnSetAndCancels(t *testing.T) {
	cfg := newFeatureConfig(t)

	var got []Change
	cancel := cfg.OnChange(func(changes []Change) { got = append(got, changes...) })

	if err := cfg.Set("port", 1234); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(got) != 1 || got[0].Key != "port" {
		t.Fatalf("callback got %v, want one change for port", got)
	}
	if got[0].New != 1234 || got[0].Source != SourceSet {
		t.Errorf("change = %+v, want New=1234 Source=set", got[0])
	}

	cancel()
	got = nil
	if err := cfg.Set("port", 5678); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("callback fired after cancel: %v", got)
	}
}

func TestSentinelErrors(t *testing.T) {
	cfg := newFeatureConfig(t)

	// Unknown key via ValidateKey.
	err := cfg.ValidateKey("does-not-exist")
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("ValidateKey unknown: errors.Is(ErrUnknownKey) = false, err = %v", err)
	}

	// Validation failure matches ErrValidation and exposes the code-bearing wrapper.
	cfg.RegisterValidator("name", Custom(func(v interface{}) error {
		return errors.New("always fails")
	}))
	if err := cfg.Set("name", "x"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	verr := cfg.ValidateKey("name")
	if !errors.Is(verr, ErrValidation) {
		t.Errorf("ValidateKey invalid: errors.Is(ErrValidation) = false, err = %v", verr)
	}
}

func TestInitReturnsErrorForPointerToPointer(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &featureConfig{Name: DefaultValue("x"), Port: DefaultValue(0)}
	ptr := &cfg // **featureConfig

	err := cfg.Init(ptr)
	if err == nil {
		t.Fatal("Init with pointer-to-pointer: expected error, got nil")
	}
	if code := ErrorCode(err); code != 400 {
		t.Errorf("ErrorCode = %d, want 400; err = %v", code, err)
	}
}

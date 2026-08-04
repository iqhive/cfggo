package cfggo

import (
	"strings"
	"sync"
	"testing"

	"github.com/iqhive/cfggo/sources"
)

type secretValidationConfig struct {
	Structure
	APIKey func() string `cfggo:"api_key" secret:"true" help:"API key"`
}

// A validation failure on a secret-tagged field must not embed the real value
// in the error message (Init logs these errors, and Diagnose/Report print them).
func TestSecretValueNotLeakedInValidationErrors(t *testing.T) {
	cfg := &secretValidationConfig{}
	err := cfg.Init(cfg,
		WithoutFlags(),
		WithoutEnv(),
		WithValidation("api_key", MinLength(64)),
	)
	if err == nil {
		t.Fatal("Init: expected validation error, got nil")
	}
	if strings.Contains(err.Error(), "****") == false {
		t.Fatalf("validation error should carry the masked placeholder, got: %v", err)
	}

	cfg2 := &secretValidationConfig{}
	if err := cfg2.Init(cfg2, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg2.RegisterValidator("api_key", MinLength(64))
	if err := cfg2.Set("api_key", "hunter2-super-secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := cfg2.ValidateKey("api_key"); err != nil {
		if strings.Contains(err.Error(), "hunter2-super-secret") {
			t.Fatalf("ValidateKey error leaks secret value: %v", err)
		}
	} else {
		t.Fatal("ValidateKey: expected error, got nil")
	}

	d := cfg2.DiagnoseData()
	for _, kd := range d.Keys {
		if kd.Err != nil && strings.Contains(kd.Err.Error(), "hunter2-super-secret") {
			t.Fatalf("DiagnoseData error leaks secret value: %v", kd.Err)
		}
	}
	if report := cfg2.Report(); strings.Contains(report, "hunter2-super-secret") {
		t.Fatalf("Report() leaks secret value:\n%s", report)
	}
}

type boolFlagConfig struct {
	Structure
	Enabled func() bool `cfggo:"enabled"`
}

// NewFlag with a bool default must not overwrite a value that was already
// loaded (from a file, env, or Set).
func TestNewFlagBoolDoesNotOverwriteExistingValue(t *testing.T) {
	cfg := &boolFlagConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("enabled", true); err != nil {
		t.Fatalf("Set: %v", err)
	}

	cfg.NewFlag("enabled2", false, "another bool")
	if v, _ := cfg.Get("enabled2"); v != false {
		t.Fatalf("new bool flag default = %v, want false", v)
	}

	// Re-registering must be skipped, but more importantly a bool default must
	// not clobber the existing true value.
	cfg.NewFlag("enabled", false, "existing bool")
	if got := cfg.Enabled(); got != true {
		t.Fatal("NewFlag with bool default overwrote a Set value")
	}
}

type negativeFlagConfig struct {
	Structure
	Offset func() int `cfggo:"offset"`
}

// A negative number argument must not be treated as an unknown flag name.
func TestNegativeNumberArgsNotTreatedAsFlags(t *testing.T) {
	cfg := &negativeFlagConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if name, ok := cfg.firstUnknownFlag([]string{"-1"}); ok {
		t.Fatalf("firstUnknownFlag(-1) = %q, want no flag", name)
	}
	if name, ok := cfg.firstUnknownFlag([]string{"-2.5"}); ok {
		t.Fatalf("firstUnknownFlag(-2.5) = %q, want no flag", name)
	}

	cfg.NewFlag("offset2", 0, "offset flag")
	out := cfg.filterKnownFlags([]string{"-1", "--offset2=3"})
	if len(out) != 2 || out[0] != "-1" || out[1] != "--offset2=3" {
		t.Fatalf("filterKnownFlags mishandled negative number: %v", out)
	}
}

type reloadRaceConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"8080"`
	Name func() string `cfggo:"name" default:"svc"`
}

// A Set landing around a Reload must never be silently lost: after both
// complete, the Set value must survive (Set values are runtime overrides that
// Reload deliberately re-asserts).
func TestReloadDoesNotDropConcurrentSet(t *testing.T) {
	for i := 0; i < 50; i++ {
		cfg := &reloadRaceConfig{}
		if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
			t.Fatalf("Init: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = cfg.Set("name", "override")
		}()
		go func() {
			defer wg.Done()
			_ = cfg.Reload()
		}()
		wg.Wait()

		if got := cfg.Name(); got != "override" {
			t.Fatalf("iteration %d: Set value lost across Reload: name = %q", i, got)
		}
	}
}

// HandlerEnv must refuse an empty prefix rather than importing the entire
// process environment (which may contain unrelated secrets).
func TestHandlerEnvRejectsEmptyPrefix(t *testing.T) {
	if _, err := sources.NewHandlerEnv("", false).LoadConfig(); err == nil {
		t.Fatal("LoadConfig() with empty prefix: expected error, got nil")
	}
}

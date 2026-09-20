package cfggo

import (
	"errors"
	"strings"
	"testing"
)

// Redaction fixtures: a secret-tagged URL field and a non-secret control.
// The secret value is crafted so url.Parse fails while echoing the input
// (missing ']' in host), exercising validators that embed the checked value.
type urlRedactConfig struct {
	Structure
	Endpoint func() string `cfggo:"endpoint" secret:"true"`
	Site     func() string `cfggo:"site"`
}

const (
	redactSecretURL = "http://[::1-supersecret99"
	redactMarker    = "supersecret99"
)

func initURLRedactConfig(t *testing.T, secretValue, siteValue string) *urlRedactConfig {
	t.Helper()
	cfg := &urlRedactConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("endpoint", secretValue); err != nil {
		t.Fatalf("Set endpoint: %v", err)
	}
	if err := cfg.Set("site", siteValue); err != nil {
		t.Fatalf("Set site: %v", err)
	}
	cfg.RegisterValidator("endpoint", URL())
	cfg.RegisterValidator("site", URL())
	return cfg
}

func TestSecretURLValidationRedacted(t *testing.T) {
	cfg := initURLRedactConfig(t, redactSecretURL, "https://example.com")

	t.Run("validate", func(t *testing.T) {
		err := cfg.Validate()
		if err == nil {
			t.Fatal("Validate: expected error, got nil")
		}
		if strings.Contains(err.Error(), redactMarker) {
			t.Fatalf("Validate leaks secret value: %v", err)
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("Validate: errors.Is(ErrValidation) = false, err = %v", err)
		}
	})

	t.Run("validate_key", func(t *testing.T) {
		err := cfg.ValidateKey("endpoint")
		if err == nil {
			t.Fatal("ValidateKey: expected error, got nil")
		}
		if strings.Contains(err.Error(), redactMarker) {
			t.Fatalf("ValidateKey leaks secret value: %v", err)
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("ValidateKey: errors.Is(ErrValidation) = false, err = %v", err)
		}
	})

	t.Run("diagnose_data", func(t *testing.T) {
		d := cfg.DiagnoseData()
		for _, kd := range d.Keys {
			if kd.Err != nil && strings.Contains(kd.Err.Error(), redactMarker) {
				t.Fatalf("DiagnoseData error leaks secret value: %v", kd.Err)
			}
		}
	})

	t.Run("report", func(t *testing.T) {
		if report := cfg.Report(); strings.Contains(report, redactMarker) {
			t.Fatalf("Report() leaks secret value:\n%s", report)
		}
	})

	t.Run("explain_key", func(t *testing.T) {
		if out := cfg.ExplainKey("endpoint"); strings.Contains(out, redactMarker) {
			t.Fatalf("ExplainKey() leaks secret value:\n%s", out)
		}
	})
}

// Non-secret control: validation detail must NOT be swallowed for ordinary keys.
func TestNonSecretURLValidationNotSwallowed(t *testing.T) {
	cfg := initURLRedactConfig(t, "https://example.com", "not-a-url")

	err := cfg.ValidateKey("site")
	if err == nil {
		t.Fatal("ValidateKey(site): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "scheme and host") {
		t.Fatalf("non-secret validation detail swallowed, got: %v", err)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("non-secret: errors.Is(ErrValidation) = false, err = %v", err)
	}
}

type secretIntReloadConfig struct {
	Structure
	Attempts func() int `cfggo:"attempts" secret:"true"`
}

// The applyLoaded path (reload runtime-override restore, dynamicVar.Set) must
// redact conversion errors for secret keys.
func TestSecretApplyLoadedRedacted(t *testing.T) {
	cfg := &secretIntReloadConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	t.Run("apply_loaded", func(t *testing.T) {
		err := cfg.applyLoaded("attempts", "reload-supersecret99", SourceSet)
		if err == nil {
			t.Fatal("applyLoaded: expected conversion error, got nil")
		}
		if strings.Contains(err.Error(), "reload-supersecret99") {
			t.Fatalf("applyLoaded leaks secret value: %v", err)
		}
	})
}

// dynamicVar is bound to real config keys (env layer), so a secret-bound
// dynamicVar must not expose the credential via String().
func TestSecretDynamicVarStringMasked(t *testing.T) {
	cfg := &urlRedactConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("endpoint", "https://hunter2-secret-value"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	dv := &dynamicVar{config: &cfg.Structure, name: "endpoint"}
	if got := dv.String(); got != maskedValue {
		t.Fatalf("dynamicVar.String() for secret key = %q, want %q", got, maskedValue)
	}
}

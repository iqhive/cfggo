package cfggo

import (
	"strings"
	"testing"
)

type secretSetConfig struct {
	Structure
	Pin  func() int `cfggo:"pin" secret:"true"`
	Port func() int `cfggo:"port"`
}

func TestSetErrorsAreRedactedForSecretKeys(t *testing.T) {
	cfg := &secretSetConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg.RegisterValidator("pin", Range(1000, 9999))

	err := cfg.Set("pin", "not-a-number-RAW")
	if err == nil {
		t.Fatal("Set accepted a non-numeric pin")
	}
	if strings.Contains(err.Error(), "not-a-number-RAW") {
		t.Errorf("conversion error echoes the secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "redacted") {
		t.Errorf("conversion error = %v, want it to say the value was redacted", err)
	}

	err = cfg.Set("pin", 12)
	if err == nil {
		t.Fatal("Set accepted an out-of-range pin")
	}
	if strings.Contains(err.Error(), "value=12") || !strings.Contains(err.Error(), "value=****") {
		t.Errorf("validation error = %v, want the value masked", err)
	}

	// Non-secret keys keep the detail that makes the error useful
	err = cfg.Set("port", "not-a-number")
	if err == nil || !strings.Contains(err.Error(), `"not-a-number"`) {
		t.Errorf("Set(port) error = %v, want the rejected value named", err)
	}
}

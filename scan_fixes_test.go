package cfggo

import (
	"errors"
	"strings"
	"testing"
)

type secretInvalidDefaultScanConfig struct {
	Structure
	Token func() int `cfggo:"token" secret:"true" default:"prod-cred-9f8e7d6c5b4a"`
}

type plainInvalidDefaultScanConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"not-a-number"`
}

// A secret-tagged field with an unconvertible default must fail Init without
// quoting the raw default value in the returned error.
func TestSecretInvalidDefaultTagRedacted(t *testing.T) {
	setProcessArgs(t)
	cfg := &secretInvalidDefaultScanConfig{}
	err := cfg.Init(cfg, WithoutFlags(), WithoutEnv())
	if err == nil {
		t.Fatal("Init: expected invalid default tag error, got nil")
	}
	if strings.Contains(err.Error(), "prod-cred-9f8e7d6c5b4a") {
		t.Fatalf("Init error leaks secret default value: %v", err)
	}
	if !strings.Contains(err.Error(), `"token"`) {
		t.Fatalf("Init error = %q, want key context", err.Error())
	}
}

// Control case: a non-secret field with an unconvertible default must still
// report the raw value so the misconfiguration is debuggable.
func TestNonSecretInvalidDefaultTagKeepsValue(t *testing.T) {
	setProcessArgs(t)
	cfg := &plainInvalidDefaultScanConfig{}
	err := cfg.Init(cfg, WithoutFlags(), WithoutEnv())
	if err == nil {
		t.Fatal("Init: expected invalid default tag error, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-number") {
		t.Fatalf("Init error = %q, want raw default value for non-secret key", err.Error())
	}
}

// Contract for the helper reused by the secret-key Warn paths: secret-key
// errors are replaced, non-secret errors pass through untouched, nil stays nil.
func TestRedactSecretValueErrorContract(t *testing.T) {
	setProcessArgs(t)
	cfg := &secretFlagConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	raw := errors.New(`cannot parse int "hunter2-secret"`)
	if got := cfg.redactSecretValueError("api.key", raw); strings.Contains(got.Error(), "hunter2-secret") {
		t.Fatalf("secret key error not redacted: %v", got)
	}
	if got := cfg.redactSecretValueError("port", raw); got != raw {
		t.Fatalf("non-secret key error was modified: %v", got)
	}
	if cfg.redactSecretValueError("api.key", nil) != nil {
		t.Fatal("nil error should stay nil")
	}
}

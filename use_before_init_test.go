package cfggo

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

type beforeInitConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestUseBeforeInitDoesNotPoisonLaterInit(t *testing.T) {
	setProcessArgs(t)
	var logs bytes.Buffer
	SetLogOutput(&logs)
	t.Cleanup(func() { SetLogOutput(os.Stderr) })

	cfg := &beforeInitConfig{}
	if v, ok := cfg.Get("port"); ok {
		t.Fatalf("Get before Init = %v, want not found", v)
	}
	if err := cfg.Set("port", 1); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Set before Init = %v, want ErrUnknownKey", err)
	}
	if got := cfg.String(); !strings.HasSuffix(got, ":\n") {
		t.Fatalf("String() before Init = %q, want an empty dump", got)
	}
	_ = cfg.Explain()
	if d := cfg.DiagnoseData(); len(d.Keys) != 0 {
		t.Fatalf("DiagnoseData before Init has %d keys, want none", len(d.Keys))
	}
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload before Init = %v, want nil", err)
	}
	if cfg.Port != nil {
		t.Fatal("accessor was wired without Init")
	}
	if !strings.Contains(logs.String(), "before Init") {
		t.Fatalf("expected a warning about use before Init, got:\n%s", logs.String())
	}

	// The real Init must still succeed and wire everything up
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init after early use = %v", err)
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want 8080", got)
	}
	if v, ok := cfg.Get("port"); !ok || v != 8080 {
		t.Fatalf("Get(port) = %v, %v", v, ok)
	}
	if err := cfg.Set("port", 9090); err != nil || cfg.Port() != 9090 {
		t.Fatalf("Set after Init: %v, Port() = %d", err, cfg.Port())
	}
	if n := strings.Count(logs.String(), "before Init"); n != 1 {
		t.Fatalf("warning logged %d times, want once", n)
	}
}

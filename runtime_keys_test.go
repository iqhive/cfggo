package cfggo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/cfglogger"
)

type runtimeKeyConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"1"`
}

func TestNewFlagBeforeInitTakesPartInEveryLayer(t *testing.T) {
	setProcessArgs(t, "--extra=40")
	t.Setenv("EXTRA", "30")
	t.Setenv("LABEL", "from-env")
	handler := &countingHandler{data: json.RawMessage(`{"extra": 20, "label": "from-file"}`)}
	var logs bytes.Buffer
	cfg := &runtimeKeyConfig{}
	cfg.NewFlag("extra", 10, "an extra integer")
	cfg.NewFlag("label", "", "a label")
	cfg.NewFlag("any", nil, "inferred")
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithStrictKeys(),
		WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs))); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := MustValue[int](&cfg.Structure, "extra"); got != 40 {
		t.Errorf("extra = %d, want 40 from the flag (over env 30 and file 20)", got)
	}
	if src, _ := cfg.Source("extra"); src != SourceFlag {
		t.Errorf("Source(extra) = %v, want flag", src)
	}
	if got := MustValue[string](&cfg.Structure, "label"); got != "from-env" {
		t.Errorf("label = %q, want the env value over the file value", got)
	}
	if strings.Contains(logs.String(), "unrecognized") || strings.Contains(logs.String(), "already registered") || strings.Contains(logs.String(), "before Init") {
		t.Errorf("unexpected warnings for runtime keys:\n%s", logs.String())
	}
	d := cfg.DiagnoseData()
	if len(d.Unrecognized) != 0 {
		t.Errorf("Unrecognized = %v, want none", d.Unrecognized)
	}
	for _, k := range d.Keys {
		if k.Key == "extra" && (!k.Recognized || k.Help != "an extra integer" || k.Type != "int") {
			t.Errorf("Diagnose(extra) = %+v, want recognized with help and type", k)
		}
	}
	if !strings.Contains(cfg.String(), "an extra integer") {
		t.Errorf("String() should carry the NewFlag help:\n%s", cfg.String())
	}

	// Set, Save and Reload treat it like any other key
	if err := cfg.Set("extra", 41); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if saved := string(handler.saved[0]); !strings.Contains(saved, `"extra":41`) || !strings.Contains(saved, `"label":"from-file"`) {
		t.Errorf("saved = %s, want the Set value and the file value (env override omitted)", saved)
	}
	if err := cfg.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := MustValue[int](&cfg.Structure, "extra"); got != 41 {
		t.Errorf("extra after reload = %d, want the Set value re-asserted", got)
	}
}

func TestNewFlagAfterInit(t *testing.T) {
	setProcessArgs(t)
	t.Setenv("LATE", "7")
	var logs bytes.Buffer
	cfg := &runtimeKeyConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithStrictKeys(),
		WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs))); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg.NewFlag("late", 5, "added after Init")
	if got := MustValue[int](&cfg.Structure, "late"); got != 7 {
		t.Errorf("late = %d, want the env value read when the key was added", got)
	}
	cfg.NewFlag("plain", "x", "")
	if got, ok := cfg.Get("plain"); !ok || got != "x" {
		t.Errorf("Get(plain) = %v, %v", got, ok)
	}
	if err := cfg.Set("plain", "y"); err != nil || MustValue[string](&cfg.Structure, "plain") != "y" {
		t.Errorf("Set(plain): %v", err)
	}
	if d := cfg.DiagnoseData(); len(d.Unrecognized) != 0 || !d.Valid {
		t.Errorf("Diagnose after NewFlag: unrecognized=%v valid=%v", d.Unrecognized, d.Valid)
	}
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := MustValue[int](&cfg.Structure, "late"); got != 7 {
		t.Errorf("late after reload = %d, want 7 (default seeded, env re-applied)", got)
	}
	if strings.Contains(logs.String(), "before Init") {
		t.Errorf("NewFlag after Init logged a use-before-Init warning:\n%s", logs.String())
	}
}

func TestNewFlagNilDefaultInfersFromText(t *testing.T) {
	setProcessArgs(t, "--count=3")
	t.Setenv("ANY", "hello")
	cfg := &runtimeKeyConfig{}
	cfg.NewFlag("any", nil, "")
	cfg.NewFlag("count", nil, "")
	if err := cfg.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got, _ := cfg.Get("any"); got != "hello" {
		t.Errorf("any = %#v, want the env string", got)
	}
	if got, _ := cfg.Get("count"); got != 3 {
		t.Errorf("count = %#v (%T), want int 3 inferred from the flag", got, got)
	}
}

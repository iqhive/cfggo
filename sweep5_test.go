package cfggo

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/cfglogger"
)

type toggle bool

type namedBoolConfig struct {
	Structure
	Enabled func() toggle            `cfggo:"enabled"`
	Flags   func() []toggle          `cfggo:"flags"`
	Toggles func() map[string]toggle `cfggo:"toggles"`
}

func TestNamedBoolCollectionsFromStrings(t *testing.T) {
	setProcessArgs(t, "--flags=true,no", "--toggles=a:yes,b:0")
	t.Setenv("ENABLED", "true")
	cfg := &namedBoolConfig{}
	if err := cfg.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if v, _ := cfg.Get("enabled"); v != toggle(true) {
		t.Errorf("Get(enabled) = %#v (%T), want toggle(true)", v, v)
	}
	if got := cfg.Flags(); len(got) != 2 || got[0] != true || got[1] != false {
		t.Errorf("Flags() = %v, want [true false]", got)
	}
	if got := cfg.Toggles(); got["a"] != true || got["b"] != false {
		t.Errorf("Toggles() = %v, want a:true b:false", got)
	}
	if err := cfg.Set("flags", "t,f"); err != nil || cfg.Flags()[1] != false {
		t.Errorf("Set(flags) = %v, Flags() = %v", err, cfg.Flags())
	}
}

type byteSliceConfig struct {
	Structure
	Key func() []byte `cfggo:"key"`
}

func TestByteSliceRoundTripsThroughSave(t *testing.T) {
	setProcessArgs(t)
	handler := &countingHandler{data: []byte(`{}`)}
	cfg := &byteSliceConfig{Key: DefaultValue([]byte("hello"))}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("key", []byte("secret-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	handler.setData(string(handler.saved[0]))
	again := &byteSliceConfig{}
	if err := again.Init(again, WithConfigHandler(handler), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init from the saved document %s: %v", handler.saved[0], err)
	}
	if got := string(again.Key()); got != "secret-bytes" {
		t.Fatalf("Key() after round trip = %q", got)
	}
}

func TestByteSliceTextForms(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("from-env"))
	t.Setenv("KEY", encoded)
	setProcessArgs(t, "--key="+strings.TrimRight(encoded, "="))
	cfg := &byteSliceConfig{}
	if err := cfg.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// the flag (unpadded base64) wins over the env (padded base64)
	if got := string(cfg.Key()); got != "from-env" {
		t.Fatalf("Key() = %q, want the decoded flag value", got)
	}
	if err := cfg.Set("key", "[104,105]"); err != nil || string(cfg.Key()) != "hi" {
		t.Fatalf("JSON array form: err=%v Key()=%q", err, cfg.Key())
	}
	err := cfg.Set("key", "not base64!")
	if err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("Set(raw text) = %v, want an error explaining the base64 form", err)
	}
	if got := string(cfg.Key()); got != "hi" {
		t.Fatalf("Key() after a rejected Set = %q, want unchanged", got)
	}
}

func TestByteSliceInvalidTextFailsInitStrictly(t *testing.T) {
	setProcessArgs(t)
	t.Setenv("KEY", "definitely not base64!")
	cfg := &byteSliceConfig{}
	err := cfg.Init(cfg, WithoutFlags())
	if !errors.Is(err, ErrSource) {
		t.Fatalf("Init = %v, want ErrSource for a []byte value that is not base64", err)
	}
}

func TestDroppedFlagAndPositionalLogsDoNotEchoValues(t *testing.T) {
	setProcessArgs(t, "--other-token=SECRET-VALUE", "--other", "ALSO-SECRET", "--port=5", "stray-SECRET")
	var logs bytes.Buffer
	logger := cfglogger.NewDefaultLoggerWithWriter(&logs)
	logger.SetLevel(-8)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithIgnoreUnknownVars(), WithoutEnv(), WithLogger(logger)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 5 {
		t.Fatalf("Port() = %d", got)
	}
	out := logs.String()
	for _, secret := range []string{"SECRET-VALUE", "ALSO-SECRET", "stray-SECRET"} {
		if strings.Contains(out, secret) {
			t.Errorf("logs echo %q:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "flag=other-token") || !strings.Contains(out, "flag=other") {
		t.Errorf("logs should still name the dropped flags:\n%s", out)
	}
	if !strings.Contains(out, "count=1") {
		t.Errorf("positional warning should report a count:\n%s", out)
	}
	_ = os.Args
}

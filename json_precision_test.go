package cfggo

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type precisionConfig struct {
	Structure
	ID       func() int64                  `cfggo:"id"`
	Big      func() uint64                 `cfggo:"big"`
	Timeout  func() time.Duration          `cfggo:"timeout"`
	Count    func() int                    `cfggo:"count"`
	Ratio    func() float64                `cfggo:"ratio"`
	Anything func() interface{}            `cfggo:"anything"`
	Labels   func() map[string]interface{} `cfggo:"labels"`
	Name     func() string                 `cfggo:"name"`
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return file
}

func TestFileLoadPreservesLargeIntegers(t *testing.T) {
	file := writeConfigFile(t, `{
		"id": 9007199254740993,
		"big": 18446744073709551615,
		"timeout": 1500000000,
		"count": 1e3,
		"ratio": 0.5,
		"anything": 9007199254740993,
		"labels": {"n": 9007199254740993, "small": 7, "list": [9007199254740993, 2]},
		"extra": 9007199254740993,
		"extra_small": 42
	}`)
	cfg := &precisionConfig{}
	if err := cfg.Init(cfg, WithFileConfig(file), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.ID(); got != 9007199254740993 {
		t.Errorf("ID() = %d, want 9007199254740993 (precision lost through float64)", got)
	}
	if got := cfg.Big(); got != math.MaxUint64 {
		t.Errorf("Big() = %d, want MaxUint64", got)
	}
	if got := cfg.Timeout(); got != 1500*time.Millisecond {
		t.Errorf("Timeout() = %v, want 1.5s (JSON number is nanoseconds)", got)
	}
	if got := cfg.Count(); got != 1000 {
		t.Errorf("Count() = %d, want 1000 from the exponent literal 1e3", got)
	}
	if got := cfg.Ratio(); got != 0.5 {
		t.Errorf("Ratio() = %v, want 0.5", got)
	}
	if got, ok := cfg.Anything().(int64); !ok || got != 9007199254740993 {
		t.Errorf("Anything() = %#v, want exact int64", cfg.Anything())
	}

	labels := cfg.Labels()
	if got, ok := labels["n"].(int64); !ok || got != 9007199254740993 {
		t.Errorf(`Labels()["n"] = %#v, want exact int64`, labels["n"])
	}
	// Ordinary integers keep the float64 dynamic type encoding/json produces,
	// so the type callers see for everyday values is unchanged
	if got, ok := labels["small"].(float64); !ok || got != 7 {
		t.Errorf(`Labels()["small"] = %#v, want float64(7)`, labels["small"])
	}
	list, _ := labels["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf(`Labels()["list"] = %#v, want two elements`, labels["list"])
	}
	if got, ok := list[0].(int64); !ok || got != 9007199254740993 {
		t.Errorf("list[0] = %#v, want exact int64", list[0])
	}
	if got, ok := list[1].(float64); !ok || got != 2 {
		t.Errorf("list[1] = %#v, want float64(2)", list[1])
	}

	// Unrecognised (untyped) keys never hold a json.Number
	if extra, _ := cfg.Get("extra"); extra != int64(9007199254740993) {
		t.Errorf(`Get("extra") = %#v, want int64`, extra)
	}
	if small, _ := cfg.Get("extra_small"); small != float64(42) {
		t.Errorf(`Get("extra_small") = %#v, want float64(42)`, small)
	}

	// The exact values survive a save/load round trip
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	again := &precisionConfig{}
	if err := again.Init(again, WithFileConfig(file), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init after save: %v", err)
	}
	if again.ID() != 9007199254740993 || again.Big() != math.MaxUint64 {
		t.Errorf("round trip: ID() = %d, Big() = %d", again.ID(), again.Big())
	}
}

func TestFileLoadReportsUnrepresentableNumbers(t *testing.T) {
	cfg := &precisionConfig{}
	err := cfg.Init(cfg, WithFileConfig(writeConfigFile(t, `{"count": 1e30}`)), WithoutFlags(), WithoutEnv())
	if err == nil {
		t.Fatal("Init succeeded, want an overflow error for count=1e30")
	}
	if !errors.Is(err, ErrSource) {
		t.Fatalf("Init error = %v, want ErrSource", err)
	}
}

func TestFileLoadRejectsTrailingData(t *testing.T) {
	cfg := &precisionConfig{}
	err := cfg.Init(cfg, WithFileConfig(writeConfigFile(t, `{"count": 1} {"count": 2}`)), WithoutFlags(), WithoutEnv())
	if !errors.Is(err, ErrSource) {
		t.Fatalf("Init error = %v, want ErrSource for trailing data after the document", err)
	}
	if cfg.Count() != 0 {
		t.Fatalf("Count() = %d after a failed load, want the default 0", cfg.Count())
	}
}

type logLevel string

func TestSetAcceptsNamedStringTypesAndRejectsIntegersForStrings(t *testing.T) {
	cfg := &precisionConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("name", logLevel("info")); err != nil {
		t.Fatalf("Set(named string): %v", err)
	}
	if got := cfg.Name(); got != "info" {
		t.Fatalf("Name() = %q, want %q", got, "info")
	}
	if err := cfg.Set("count", logLevel("42")); err != nil {
		t.Fatalf("Set(named string into int): %v", err)
	}
	if got := cfg.Count(); got != 42 {
		t.Fatalf("Count() = %d, want 42", got)
	}
	// reflect would happily convert 65 to the rune "A"; a scalar becomes its
	// literal text instead
	if err := cfg.Set("name", 65); err != nil || cfg.Name() != "65" {
		t.Fatalf("Set(65) into a string key: err=%v Name()=%q, want \"65\"", err, cfg.Name())
	}
}

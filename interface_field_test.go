package cfggo

import (
	"reflect"
	"testing"
)

type interfaceFieldConfig struct {
	Structure
	Any   func() interface{} `cfggo:"any"`
	Other func() interface{} `cfggo:"other"`
	Third func() interface{} `cfggo:"third"`
}

func TestInterfaceFieldAcceptsEnvAndFlagValues(t *testing.T) {
	setProcessArgs(t, "--other=42", "--third=[1,2]")
	t.Setenv("ANY", "hello")
	cfg := &interfaceFieldConfig{}
	if err := cfg.Init(cfg); err != nil {
		t.Fatalf("Init: %v (an interface{} field with no default must accept env and flag values)", err)
	}
	if got := cfg.Any(); got != "hello" {
		t.Errorf("Any() = %#v, want the env string", got)
	}
	if got := cfg.Other(); got != 42 {
		t.Errorf("Other() = %#v (%T), want int 42 inferred from the flag", got, got)
	}
	if got, want := cfg.Third(), []interface{}{float64(1), float64(2)}; !reflect.DeepEqual(got, want) {
		t.Errorf("Third() = %#v, want a decoded JSON array", got)
	}
	if src, _ := cfg.Source("any"); src != SourceEnv {
		t.Errorf("Source(any) = %v, want env", src)
	}
}

func TestInterfaceFieldFromJSONEnvValue(t *testing.T) {
	setProcessArgs(t)
	t.Setenv("ANY", `{"n": 9007199254740993, "s": "x"}`)
	cfg := &interfaceFieldConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got, ok := cfg.Any().(map[string]interface{})
	if !ok {
		t.Fatalf("Any() = %#v, want a decoded object", cfg.Any())
	}
	if got["n"] != int64(9007199254740993) || got["s"] != "x" {
		t.Errorf("Any() = %#v, want exact integer and string members", got)
	}
}

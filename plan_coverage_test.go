package cfggo

import (
	"testing"
	"time"
)

type numericKindsConfig struct {
	Structure
	I8  func() int8          `cfggo:"i8"`
	I16 func() int16         `cfggo:"i16"`
	I32 func() int32         `cfggo:"i32"`
	U   func() uint          `cfggo:"u"`
	U8  func() uint8         `cfggo:"u8"`
	U16 func() uint16        `cfggo:"u16"`
	U32 func() uint32        `cfggo:"u32"`
	U64 func() uint64        `cfggo:"u64"`
	F32 func() float32       `cfggo:"f32"`
	F64 func() float64       `cfggo:"f64"`
	Dur func() time.Duration `cfggo:"dur"`
}

func TestNumericAccessorKinds(t *testing.T) {
	cfg := &numericKindsConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if got := cfg.I8(); got != 0 {
		t.Errorf("I8() = %d, want 0", got)
	}
	if got := cfg.I16(); got != 0 {
		t.Errorf("I16() = %d, want 0", got)
	}
	if got := cfg.I32(); got != 0 {
		t.Errorf("I32() = %d, want 0", got)
	}
	if got := cfg.U(); got != 0 {
		t.Errorf("U() = %d, want 0", got)
	}
	if got := cfg.U8(); got != 0 {
		t.Errorf("U8() = %d, want 0", got)
	}
	if got := cfg.U16(); got != 0 {
		t.Errorf("U16() = %d, want 0", got)
	}
	if got := cfg.U32(); got != 0 {
		t.Errorf("U32() = %d, want 0", got)
	}
	if got := cfg.U64(); got != 0 {
		t.Errorf("U64() = %d, want 0", got)
	}
	if got := cfg.F32(); got != 0 {
		t.Errorf("F32() = %f, want 0", got)
	}
	if got := cfg.F64(); got != 0 {
		t.Errorf("F64() = %f, want 0", got)
	}
	if got := cfg.Dur(); got != 0 {
		t.Errorf("Dur() = %v, want 0", got)
	}

	// Exercise the readTyped slow path: a value stored as a different numeric
	// width must be converted on read.
	if err := cfg.Set("i8", 7); err != nil {
		t.Fatalf("Set(i8) error = %v", err)
	}
	if got := cfg.I8(); got != 7 {
		t.Errorf("I8() = %d, want 7", got)
	}

	// readTyped conversion branch: store an int under an int8 accessor key.
	cfg.configMutex.Lock()
	cfg.configData["i8"] = 9
	cfg.configData["u8"] = nil
	cfg.configMutex.Unlock()
	if got := cfg.I8(); got != 9 {
		t.Errorf("I8() after cross-width store = %d, want 9", got)
	}
	if got := cfg.U8(); got != 0 {
		t.Errorf("U8() after nil store = %d, want 0", got)
	}
}

func TestCloneBoolMap(t *testing.T) {
	if got := cloneBoolMap(nil); got != nil {
		t.Fatalf("cloneBoolMap(nil) = %#v, want nil", got)
	}

	in := map[string]bool{"a": true, "b": false}
	out := cloneBoolMap(in)
	if len(out) != 2 || !out["a"] || out["b"] {
		t.Fatalf("cloneBoolMap() = %#v, want a copy of %#v", out, in)
	}
	out["c"] = true
	if _, exists := in["c"]; exists {
		t.Fatal("cloneBoolMap() returned a map aliasing the input")
	}
}

type suspectTagConfig struct {
	Structure
	Port   func() int `cfggo:"port"`
	Plain  int        `cfg:"plain"`
	Config int        `config:"config"`
}

func TestHasExplicitConfigTagSuspectFields(t *testing.T) {
	cfg := &suspectTagConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got := cfg.Port(); got != 0 {
		t.Errorf("Port() = %d, want 0", got)
	}
}

func TestValueBranchCoverage(t *testing.T) {
	cfg := &numericKindsConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// raw is a T and non-nil: covered elsewhere, but assert the convertible
	// path here by storing a float64 and reading as int.
	if err := cfg.Set("i16", 42); err != nil {
		t.Fatalf("Set(i16) error = %v", err)
	}
	if got, ok := Value[int16](&cfg.Structure, "i16"); !ok || got != 42 {
		t.Fatalf("Value[int16](i16) = %d, %v; want 42, true", got, ok)
	}

	// non-convertible: string stored under an int-typed key, read as int.
	cfg.configMutex.Lock()
	cfg.configData["i32"] = "not-an-int"
	cfg.configMutex.Unlock()
	if got, ok := Value[int32](&cfg.Structure, "i32"); ok || got != 0 {
		t.Fatalf("Value[int32](i32) = %d, %v; want 0, false", got, ok)
	}

	// absent key.
	if got, ok := Value[uint](&cfg.Structure, "absent"); ok || got != 0 {
		t.Fatalf("Value[uint](absent) = %d, %v; want 0, false", got, ok)
	}

	// nil raw value read as a type: store a nil interface value.
	cfg.configMutex.Lock()
	cfg.configData["nilkey"] = nil
	cfg.configMutex.Unlock()
	if got, ok := Value[string](&cfg.Structure, "nilkey"); ok || got != "" {
		t.Fatalf("Value[string](nilkey) = %q, %v; want \"\", false", got, ok)
	}
}

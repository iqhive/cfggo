package cfggo_test

import (
	"errors"
	"testing"

	"github.com/iqhive/cfggo"
)

type newFlagValidatorConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"8080"`
}

// A validator may target a key registered with NewFlag: such a key takes part
// in every layer like a struct-backed key, so the configuration-shape check
// must not reject it as unknown.
func TestValidatorForNewFlagKeyIsAccepted(t *testing.T) {
	cfg := &newFlagValidatorConfig{}
	cfg.NewFlag("region", "eu", "deployment region")
	err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(),
		cfggo.WithValidation("region", cfggo.OneOf("eu", "us")))
	if err != nil {
		t.Fatalf("Init rejected a validator for a NewFlag key: %v", err)
	}
	if err := cfg.Set("region", "asia"); err == nil {
		t.Fatal("Set bypassed the validator registered for the NewFlag key")
	} else if !errors.Is(err, cfggo.ErrValidation) {
		t.Fatalf("Set error = %v, want a validation error", err)
	}
	if err := cfg.ValidateConfigShape(); err != nil {
		t.Fatalf("ValidateConfigShape: %v", err)
	}

	// A genuinely unknown key is still reported
	cfg.RegisterValidator("nope", cfggo.Required())
	if err := cfg.ValidateConfigShape(); err == nil || !errors.Is(err, cfggo.ErrUnknownKey) {
		t.Fatalf("ValidateConfigShape error = %v, want ErrUnknownKey", err)
	}
}

// Value[T] must convert with the same rules as Set: an integer read as a
// string is its decimal text, never the rune with that code point, and an
// out-of-range integer read as a narrower type reports !ok instead of wrapping.
func TestValueConvertsLikeSet(t *testing.T) {
	cfg := &newFlagValidatorConfig{}
	cfg.NewFlag("dyn", nil, "untyped key")
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("dyn", 65); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := cfggo.Value[string](&cfg.Structure, "dyn"); !ok || got != "65" {
		t.Fatalf("Value[string](65) = %q, %v; want \"65\", true", got, ok)
	}
	if err := cfg.Set("dyn", 300); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := cfggo.Value[int8](&cfg.Structure, "dyn"); ok || got != 0 {
		t.Fatalf("Value[int8](300) = %d, %v; want 0, false", got, ok)
	}
	if got, ok := cfggo.Value[int64](&cfg.Structure, "dyn"); !ok || got != 300 {
		t.Fatalf("Value[int64](300) = %d, %v; want 300, true", got, ok)
	}
}

package cfggo

import (
	"strings"
	"testing"

	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/validcfg"
)

type renderSafetyConfig struct {
	Structure
	Data func() interface{} `cfggo:"data" help:"arbitrary value"`
	Port func() int         `cfggo:"port"`
}

// A value set through Set (or a NewFlag default) can be a cyclic container:
// the deep clone deliberately preserves cycles. fmt's %v formats cyclic maps,
// slices, and the interfaces inside them by recursing until the goroutine
// stack overflows, which is a fatal error no recover can catch — it kills the
// whole process. Every cfggo path that renders a stored value used fmt, so a
// single cyclic value made String(), DiagnoseData(), Report(), ExplainKey(),
// and validation errors destroy the process. These tests render cyclic values
// through all of those paths; before the fix each one was a fatal crash.
func TestRendersSurviveCyclicValues(t *testing.T) {
	cfg := &renderSafetyConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cyclic := map[string]interface{}{"name": "root"}
	cyclic["self"] = cyclic
	if err := cfg.Set("data", cyclic); err != nil {
		t.Fatalf("Set(cyclic map): %v", err)
	}

	t.Run("String", func(t *testing.T) {
		out := cfg.String()
		if !strings.Contains(out, "<cycle>") {
			t.Fatalf("String() = %q, want a <cycle> marker", out)
		}
		if !strings.Contains(out, "root") {
			t.Fatalf("String() = %q, want the non-cyclic entry rendered", out)
		}
	})

	t.Run("Diagnostics", func(t *testing.T) {
		out := cfg.DiagnoseData().String()
		if !strings.Contains(out, "<cycle>") {
			t.Fatalf("Diagnostics.String() = %q, want a <cycle> marker", out)
		}
	})

	t.Run("Report", func(t *testing.T) {
		out := cfg.Report()
		if !strings.Contains(out, "<cycle>") {
			t.Fatalf("Report() = %q, want a <cycle> marker", out)
		}
	})

	t.Run("ExplainKey", func(t *testing.T) {
		out := cfg.ExplainKey("data")
		if !strings.Contains(out, "<cycle>") {
			t.Fatalf("ExplainKey() = %q, want a <cycle> marker", out)
		}
	})

	t.Run("ValidationError", func(t *testing.T) {
		err := validcfg.ValidationError{Key: "data", Err: errBooster("bad")}.WithProvenance(cyclic, "set")
		msg := err.Error()
		if !strings.Contains(msg, "<cycle>") {
			t.Fatalf("ValidationError.Error() = %q, want a <cycle> marker", msg)
		}
	})

	t.Run("logger attribute", func(t *testing.T) {
		var sb strings.Builder
		cfg.SetLogger(cfglogger.NewPrintfLogger(nil, func(format string, args ...any) {
			sb.WriteString(cfglogger.FormatMessage(format, args...))
			sb.WriteByte('\n')
		}, nil, nil))
		cfg.log().Info("cfggo: rendering", "value", cyclic)
		if !strings.Contains(sb.String(), "<cycle>") {
			t.Fatalf("logged attribute = %q, want a <cycle> marker", sb.String())
		}
	})
}

// The same fatal recursion through fmt happened for a cyclic slice value
func TestRendersSurviveCyclicSlice(t *testing.T) {
	cfg := &renderSafetyConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cyclic := make([]interface{}, 1)
	cyclic[0] = cyclic
	if err := cfg.Set("data", cyclic); err != nil {
		t.Fatalf("Set(cyclic slice): %v", err)
	}

	if out := cfg.String(); !strings.Contains(out, "<cycle>") {
		t.Fatalf("String() = %q, want a <cycle> marker", out)
	}
	if out := cfg.DiagnoseData().String(); !strings.Contains(out, "<cycle>") {
		t.Fatalf("Diagnostics.String() = %q, want a <cycle> marker", out)
	}
}

// Validation output must render the offending value without crashing, and
// OneOf must build its message without crashing on a cyclic option
func TestValidationErrorsSurviveCyclicValues(t *testing.T) {
	cyclic := map[string]interface{}{"k": "v"}
	cyclic["self"] = cyclic

	validator := OneOf(cyclic)
	if err := validator(map[string]interface{}{"other": 1}); err == nil {
		t.Fatal("OneOf: expected error for non-matching value, got nil")
	} else if !strings.Contains(err.Error(), "<cycle>") {
		t.Fatalf("OneOf error = %q, want a <cycle> marker", err.Error())
	}

	cfg := &renderSafetyConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("port", 99); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// A validator that fails with a cyclic value in the message path
	cfg.RegisterValidator("data", func(value interface{}) error {
		return validcfg.ValidationError{Key: "data", Err: errBooster("nope")}.WithProvenance(cyclic, "file")
	})
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate: expected error, got nil")
	}
	if msg := err.Error(); !strings.Contains(msg, "<cycle>") {
		t.Fatalf("Validate error = %q, want a <cycle> marker", msg)
	}
}

type errBooster string

func (e errBooster) Error() string { return string(e) }

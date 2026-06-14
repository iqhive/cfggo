package validcfg

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatorsHandleNilWithoutPanic(t *testing.T) {
	validators := map[string]Validator{
		"MinLength": MinLength(1),
		"MaxLength": MaxLength(1),
		"Range":     Range(0, 10),
	}

	for name, validator := range validators {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("validator panicked on nil: %v", r)
				}
			}()
			if err := validator(nil); err == nil {
				t.Fatal("validator(nil): expected error, got nil")
			}
		})
	}
}

func TestCommonValidators(t *testing.T) {
	tests := []struct {
		name      string
		validator Validator
		value     interface{}
		wantErr   bool
	}{
		{"required string ok", Required(), "value", false},
		{"required empty string", Required(), "", true},
		{"required empty slice", Required(), []string{}, true},
		{"min length string ok", MinLength(3), "abcd", false},
		{"min length string short", MinLength(3), "ab", true},
		{"min length slice ok", MinLength(2), []int{1, 2}, false},
		{"min length wrong type", MinLength(1), 10, true},
		{"max length map ok", MaxLength(2), map[string]int{"a": 1, "b": 2}, false},
		{"max length array too long", MaxLength(2), [3]int{1, 2, 3}, true},
		{"max length wrong type", MaxLength(1), 10, true},
		{"range int ok", Range(1, 10), 5, false},
		{"range uint ok", Range(1, 10), uint(5), false},
		{"range float ok", Range(1, 10), 5.5, false},
		{"range outside", Range(1, 10), 11, true},
		{"range wrong type", Range(1, 10), "5", true},
		{"one of ok", OneOf("dev", "prod"), "prod", false},
		{"one of miss", OneOf("dev", "prod"), "test", true},
		{"regex ok", Regex(`^[a-z]+$`), "abc", false},
		{"regex miss", Regex(`^[a-z]+$`), "abc123", true},
		{"regex wrong type", Regex(`^[a-z]+$`), 123, true},
		{"email ok", Email(), "user@example.com", false},
		{"email miss", Email(), "not-email", true},
		{"url ok", URL(), "https://example.com/path", false},
		{"url missing host", URL(), "mailto:user@example.com", true},
		{"url wrong type", URL(), 123, true},
		{"custom ok", Custom(func(interface{}) error { return nil }), "anything", false},
		{"custom err", Custom(func(interface{}) error { return errors.New("boom") }), "anything", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validator(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validator(%#v) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestRegexPanicsForInvalidPattern(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Regex() with invalid pattern: expected panic, got nil")
		}
	}()
	_ = Regex("[")
}

func TestCompositeValidators(t *testing.T) {
	errRequired := errors.New("required")
	errFormat := errors.New("format")

	t.Run("all returns first failing validator", func(t *testing.T) {
		err := All(
			func(interface{}) error { return nil },
			func(interface{}) error { return errRequired },
			func(interface{}) error { return errFormat },
		)("value")
		if !errors.Is(err, errRequired) {
			t.Fatalf("All() error = %v, want %v", err, errRequired)
		}
	})

	t.Run("all passes when every validator passes", func(t *testing.T) {
		if err := All(Required(), MinLength(2))("ok"); err != nil {
			t.Fatalf("All() error = %v", err)
		}
	})

	t.Run("any passes when one validator passes", func(t *testing.T) {
		err := Any(
			func(interface{}) error { return errRequired },
			func(interface{}) error { return nil },
		)("value")
		if err != nil {
			t.Fatalf("Any() error = %v", err)
		}
	})

	t.Run("any reports aggregate when all fail", func(t *testing.T) {
		err := Any(
			func(interface{}) error { return errRequired },
			func(interface{}) error { return errFormat },
		)("value")
		if err == nil {
			t.Fatal("Any() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "none of the validators passed") {
			t.Fatalf("Any() error = %q, want aggregate message", err.Error())
		}
	})
}

func TestValidationErrorFormattingAndSentinelMatching(t *testing.T) {
	cause := errors.New("bad port")
	err := ValidationError{Key: "port", Err: cause}

	if got, want := err.Error(), "validation failed for 'port': bad port"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatal("ValidationError should match ErrValidation")
	}
	if !errors.Is(err, cause) {
		t.Fatal("ValidationError should unwrap to cause")
	}

	withProvenance := err.WithProvenance(99999, "env")
	if got, want := withProvenance.Error(), "validation failed for 'port' (value=99999, from env): bad port"; got != want {
		t.Fatalf("Error() with provenance = %q, want %q", got, want)
	}

	unknownSource := err.WithProvenance(nil, "")
	if !strings.Contains(unknownSource.Error(), "from unknown") {
		t.Fatalf("Error() with empty source = %q, want unknown source", unknownSource.Error())
	}
}

func TestValidationErrorsFormattingAndSentinelMatching(t *testing.T) {
	empty := ValidationErrors{}
	if got, want := empty.Error(), "no validation errors"; got != want {
		t.Fatalf("empty Error() = %q, want %q", got, want)
	}
	if !errors.Is(empty, ErrValidation) {
		t.Fatal("ValidationErrors should match ErrValidation")
	}

	one := ValidationErrors{{Key: "host", Err: errors.New("missing")}}
	if got, want := one.Error(), "validation failed for 'host': missing"; got != want {
		t.Fatalf("single Error() = %q, want %q", got, want)
	}

	many := ValidationErrors{
		{Key: "host", Err: errors.New("missing")},
		{Key: "port", Err: errors.New("bad")},
	}
	got := many.Error()
	for _, want := range []string{"2 validation errors occurred:", "validation failed for 'host': missing", "validation failed for 'port': bad"} {
		if !strings.Contains(got, want) {
			t.Fatalf("multi Error() = %q, want substring %q", got, want)
		}
	}
}

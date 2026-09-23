package validcfg

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidationErrorsUnwrapsToEachError(t *testing.T) {
	missing := errors.New("missing")
	badPort := errors.New("bad port")
	var err error = ValidationErrors{
		{Key: "host", Err: missing},
		{Key: "port", Err: badPort},
	}

	var ve ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As should find a ValidationError inside ValidationErrors")
	}
	if ve.Key != "host" {
		t.Fatalf("errors.As found ValidationError for %q, want first error %q", ve.Key, "host")
	}
	for _, cause := range []error{missing, badPort} {
		if !errors.Is(err, cause) {
			t.Fatalf("errors.Is(ValidationErrors, %v) = false, want true", cause)
		}
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatal("ValidationErrors should still match ErrValidation")
	}

	// The multi-error is still found as itself, and through a wrapping error
	var errs ValidationErrors
	if !errors.As(err, &errs) || len(errs) != 2 {
		t.Fatalf("errors.As(ValidationErrors) = %v, want the 2-element slice", errs)
	}
	wrapped := fmt.Errorf("init: %w", err)
	if !errors.Is(wrapped, badPort) {
		t.Fatal("errors.Is through a wrapping error should find the cause")
	}
	if !errors.As(wrapped, &ve) {
		t.Fatal("errors.As through a wrapping error should find a ValidationError")
	}

	unwrapped := err.(interface{ Unwrap() []error }).Unwrap()
	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap() returned %d errors, want 2", len(unwrapped))
	}
	if len(ValidationErrors{}.Unwrap()) != 0 {
		t.Fatal("empty ValidationErrors.Unwrap() should be empty")
	}
}

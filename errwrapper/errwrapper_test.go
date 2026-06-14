package errwrapper

import (
	"errors"
	"testing"
)

func TestDeprecatedWrapperDelegatesToCfgerror(t *testing.T) {
	cause := errors.New("boom")
	err := NewDefaultErrorWrapper()(cause, 99, "failed")

	if got, want := err.Error(), "[99] failed: boom"; got != want {
		t.Fatalf("wrapped error = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped error should unwrap to cause")
	}
	if got := Code(err); got != 99 {
		t.Fatalf("Code() = %d, want 99", got)
	}
	if got := Code(nil); got != 0 {
		t.Fatalf("Code(nil) = %d, want 0", got)
	}
}

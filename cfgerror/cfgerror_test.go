package cfgerror

import (
	"errors"
	"testing"
)

func TestErrorRenderingAndUnwrap(t *testing.T) {
	cause := errors.New("open config: permission denied")

	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "code message and cause",
			err:  &Error{Code: 7, Msg: "load failed", Err: cause},
			want: "[7] load failed: open config: permission denied",
		},
		{
			name: "message only",
			err:  &Error{Msg: "load failed"},
			want: "load failed",
		},
		{
			name: "cause only",
			err:  &Error{Err: cause},
			want: "open config: permission denied",
		},
		{
			name: "empty",
			err:  &Error{},
			want: "unspecified error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}

	wrapped := &Error{Code: 7, Msg: "load failed", Err: cause}
	if !errors.Is(wrapped, cause) {
		t.Fatal("Error should unwrap to cause")
	}
	if got := Code(wrapped); got != 7 {
		t.Fatalf("Code() = %d, want 7", got)
	}
	if got := Code(cause); got != 0 {
		t.Fatalf("Code(non cfgerror) = %d, want 0", got)
	}
	if got := Code(nil); got != 0 {
		t.Fatalf("Code(nil) = %d, want 0", got)
	}
}

func TestDefaultWrapper(t *testing.T) {
	wrapper := NewDefaultWrapper()

	err := wrapper(errors.New("boom"), 42, "operation %s failed", "load")
	if got, want := err.Error(), "[42] operation load failed: boom"; got != want {
		t.Fatalf("wrapped error = %q, want %q", got, want)
	}
	if got := Code(err); got != 42 {
		t.Fatalf("Code() = %d, want 42", got)
	}

	err = wrapper(nil, 0, "literal 100% message")
	if got, want := err.Error(), "literal 100% message"; got != want {
		t.Fatalf("literal message = %q, want %q", got, want)
	}

	if err := wrapper(nil, 0, ""); err != nil {
		t.Fatalf("empty wrapper call = %v, want nil", err)
	}
}

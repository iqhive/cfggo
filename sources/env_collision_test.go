package sources

import (
	"strings"
	"testing"
)

// Two environment variables can convert onto the same configuration key,
// because the name mapping lowercases and turns "_" into ".". Before this
// was reported, whichever variable happened to come later in os.Environ won,
// so the same process could load a different value — including a different
// secret — after a restart or a reordering by a parent process. An ambiguous
// key must be a load error naming both variables, never a silent
// last-writer-wins.
func TestHandlerEnvRejectsCollidingVariables(t *testing.T) {
	cases := []struct {
		name   string
		first  string
		second string
	}{
		{"double underscore", "MYAPP_X", "MYAPP__X"},
		{"case difference", "MYAPP_x", "MYAPP_X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.first, "value-one")
			t.Setenv(tc.second, "value-two")

			handler := NewHandlerEnv("MYAPP_", false)
			_, err := handler.LoadConfig()
			if err == nil {
				t.Fatal("LoadConfig: expected error for colliding variables, got nil")
			}
			msg := err.Error()
			for _, name := range []string{tc.first, tc.second} {
				if !strings.Contains(msg, name) {
					t.Fatalf("error %q should name the variable %q", msg, name)
				}
			}
			// The values may be credentials: they must never reach the error
			if strings.Contains(msg, "value-one") || strings.Contains(msg, "value-two") {
				t.Fatalf("error %q must not include the variable values", msg)
			}
			if !strings.Contains(msg, `"x"`) {
				t.Fatalf("error %q should name the colliding configuration key", msg)
			}
		})
	}
}

func TestHandlerEnvAcceptsDistinctVariables(t *testing.T) {
	t.Setenv("MYAPP_HOST", "example.com")
	t.Setenv("MYAPP_PORT", "8080")

	handler := NewHandlerEnv("MYAPP_", false)
	data, err := handler.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(string(data), `"host":"example.com"`) || !strings.Contains(string(data), `"port":8080`) {
		t.Fatalf("LoadConfig() = %s, want both variables loaded", string(data))
	}
}

package cfggo

import (
	"os"
	"testing"
)

// TestWithFileConfigParamName tests the WithFileConfigParamName option
func TestWithFileConfigParamName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		argName  string
		expected string
	}{
		{
			name:     "equals format",
			args:     []string{"prog", "--config=test.json"},
			argName:  "--config",
			expected: "test.json",
		},
		{
			name:     "space format",
			args:     []string{"prog", "--config", "test.json"},
			argName:  "--config",
			expected: "test.json",
		},
		{
			name:     "no match",
			args:     []string{"prog", "--other=test.json"},
			argName:  "--config",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original args
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			os.Args = tt.args

			s := &Structure{}
			opt := WithFileConfigParamName(tt.argName)
			err := opt(s)

			if err != nil {
				t.Errorf("WithFileConfigParamName() error = %v", err)
				return
			}

			if s.configHandler == nil {
				if tt.expected != "" {
					t.Error("configHandler is nil but expected a value")
				}
				return
			}

			handler, ok := s.configHandler.(*handlerFile)
			if !ok {
				t.Error("configHandler is not a handlerFile")
				return
			}

			if handler.filename != tt.expected {
				t.Errorf("filename = %v, want %v", handler.filename, tt.expected)
			}
		})
	}
}

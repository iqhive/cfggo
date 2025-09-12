package cfggo

import (
	"flag"
	"os"
	"testing"

	"github.com/iqhive/cfggo/sources"
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
			args:     []string{"prog", "--config=tests/test.json"},
			argName:  "config",
			expected: "tests/test.json",
		},
		{
			name:     "space format",
			args:     []string{"prog", "--config", "tests/test.json"},
			argName:  "config",
			expected: "tests/test.json",
		},
		{
			name:     "no match",
			args:     []string{"prog", "--other=tests/test.json"},
			argName:  "config",
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
			s.FlagSet = flag.NewFlagSet("test", flag.ContinueOnError)
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

			handler, ok := s.configHandler.(*sources.HandlerFile)
			if !ok {
				t.Error("configHandler is not a HandlerFile")
				return
			}

			if handler.Filename != tt.expected {
				t.Errorf("filename = %v, want %v", handler.Filename, tt.expected)
			}
		})
	}
}

// TestWithFileConfig tests the WithFileConfig function
func TestWithFileConfig(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		createFile     bool
		expectNoop     bool
		expectFilename string
	}{
		{
			name:           "existing file",
			filename:       "tests/test.json",
			createFile:     true,
			expectNoop:     false,
			expectFilename: "tests/test.json",
		},
		{
			name:       "non-existent file",
			filename:   "missing.json",
			createFile: false,
			expectNoop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file if needed
			if tt.createFile {
				f, err := os.Create(tt.filename)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				f.Close()
				// defer os.Remove(tt.filename)
			}

			s := &Structure{}
			opt := WithFileConfig(tt.filename)
			err := opt(s)

			if err != nil {
				t.Errorf("WithFileConfig() error = %v", err)
				return
			}

			if tt.expectNoop {
				if s.configHandler != nil {
					t.Error("Expected noop but got configHandler")
				}
				return
			}

			handler, ok := s.configHandler.(*sources.HandlerFile)
			if !ok {
				t.Error("configHandler is not a HandlerFile")
				return
			}

			if handler.Filename != tt.expectFilename {
				t.Errorf("filename = %v, want %v", handler.Filename, tt.expectFilename)
			}
		})
	}
}

// TestWithDefaultFileConfig tests the WithDefaultFileConfig function
func TestWithDefaultFileConfig(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		createFile     bool
		expectNoop     bool
		expectFilename string
	}{
		{
			name:           "existing file",
			filename:       "tests/test.json",
			createFile:     true,
			expectNoop:     false,
			expectFilename: "tests/test.json",
		},
		{
			name:       "non-existent file",
			filename:   "missing.json",
			createFile: false,
			expectNoop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file if needed
			if tt.createFile {
				f, err := os.Create(tt.filename)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				f.Close()
				// defer os.Remove(tt.filename)
			}

			s := &Structure{}
			opt := WithDefaultFileConfig(tt.filename)
			err := opt(s)

			if err != nil {
				t.Errorf("WithDefaultFileConfig() error = %v", err)
				return
			}

			if tt.expectNoop {
				if s.configHandler != nil {
					t.Error("Expected noop but got configHandler")
				}
				return
			}

			handler, ok := s.configHandler.(*sources.HandlerFile)
			if !ok {
				t.Error("configHandler is not a HandlerFile")
				return
			}

			if handler.Filename != tt.expectFilename {
				t.Errorf("filename = %v, want %v", handler.Filename, tt.expectFilename)
			}
		})
	}
}

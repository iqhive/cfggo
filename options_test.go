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
			args:     []string{"prog", "--config=testdata/test.json"},
			argName:  "config",
			expected: "testdata/test.json",
		},
		{
			name:     "space format",
			args:     []string{"prog", "--config", "testdata/test.json"},
			argName:  "config",
			expected: "testdata/test.json",
		},
		{
			name:     "no match",
			args:     []string{"prog", "--other=testdata/test.json"},
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

// TestWithFileConfigParamNameInit ensures WithFileConfigParamName works when
// applied through Init (where c.FlagSet is still nil at option-application
// time). This previously panicked with a nil pointer dereference because the
// option dereferenced c.FlagSet before Init created it.
func TestWithFileConfigParamNameInit(t *testing.T) {
	for _, args := range [][]string{
		{"prog"},                                // no config arg -> registers a String flag
		{"prog", "--config=testdata/test.json"}, // config arg present
	} {
		oldArgs := os.Args
		os.Args = args

		cfg := &Structure{}
		err := cfg.Init(cfg, WithFileConfigParamName("config"))

		os.Args = oldArgs

		if err != nil {
			t.Fatalf("Init() with WithFileConfigParamName returned error for args %v: %v", args, err)
		}
		if cfg.FlagSet == nil {
			t.Fatalf("expected FlagSet to be created for args %v", args)
		}
		if cfg.FlagSet.Lookup("config") == nil {
			t.Errorf("expected 'config' flag to be registered for args %v", args)
		}
	}
}

func TestWithFileConfigParamNameInitMissingRequiredFile(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"prog", "--config=missing-required.json"}

	cfg := &Structure{}
	err := cfg.Init(cfg, WithFileConfigParamName("config"))
	if err == nil {
		t.Fatal("Init() with missing required config file: expected error, got nil")
	}
	if code := ErrorCode(err); code != ErrCodeNotFound {
		t.Fatalf("ErrorCode = %d, want %d; err = %v", code, ErrCodeNotFound, err)
	}
}

// TestWithFileConfig tests the WithFileConfig function
func TestWithFileConfig(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		createFile     bool
		expectErr      bool
		expectFilename string
	}{
		{
			name:           "existing file",
			filename:       "testdata/test.json",
			createFile:     true,
			expectFilename: "testdata/test.json",
		},
		{
			name:      "non-existent file",
			filename:  "missing.json",
			expectErr: true,
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

			if tt.expectErr {
				if err == nil {
					t.Fatal("WithFileConfig() expected error for missing required file, got nil")
				}
				if s.configHandler != nil {
					t.Error("configHandler should remain nil when required file is missing")
				}
				return
			}
			if err != nil {
				t.Errorf("WithFileConfig() error = %v", err)
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
			filename:       "testdata/test.json",
			createFile:     true,
			expectNoop:     false,
			expectFilename: "testdata/test.json",
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

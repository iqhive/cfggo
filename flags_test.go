package cfggo

import (
	"flag"
	"os"
	"testing"
)

// TestNewFlag tests the NewFlag function to ensure that the format "--variable=value" works and that the format "variable=value" is not expected
func TestNewFlag(t *testing.T) {
	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field" config:"string_field" description:"string field"`
	}
	config := &TestConfig{
		StringField: func() string { return "default_struct_value" },
	}
	config.Init(config)

	// Test --variable=value format
	os.Args = []string{"cmd", "--string_field=flag_value"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	config.NewFlag("string_field", "default_value", "string field")
	flag.Parse()

	if config.configData["string_field"] != "flag_value" {
		t.Errorf("Expected 'flag_value', but got %v", config.configData["string_field"])
	}

	// Test --variable value format
	os.Args = []string{"cmd", "--string_field", "flag_value_space"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	config.NewFlag("string_field", "default_value", "string field")
	flag.Parse()

	if config.configData["string_field"] != "flag_value_space" {
		t.Errorf("Expected 'flag_value_space', but got %v", config.configData["string_field"])
	}
}

// TestAutoParse_NoManual ensures flags are automatically parsed if the user doesn't call flag.Parse().
func TestAutoParse_NoManual(t *testing.T) {
	// Reset any existing flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Simulate command line arguments (no manual parse call)
	os.Args = []string{"cmd", "--string_field=auto_value"}

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field" config:"string_field"`
	}

	cfg := &TestConfig{
		StringField: func() string { return "default" },
	}

	cfg.Init(cfg) // This should auto-parse flags internally

	if val := cfg.StringField(); val != "auto_value" {
		t.Errorf("Expected 'auto_value' got '%s'", val)
	}
}

// TestAutoParse_ManualBefore ensures parsing manually before Init doesn't cause errors or re-parse issues.
func TestAutoParse_ManualBefore(t *testing.T) {
	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	os.Args = []string{"cmd", "--string_field=manual_before"}
	flag.String("string_field", "defaultVal", "test field")

	// Manually parse before
	flag.Parse()

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field" config:"string_field"`
	}

	cfg := &TestConfig{
		StringField: func() string { return "default" },
	}

	cfg.Init(cfg) // Auto-parse should see it's already parsed and do nothing

	if val := cfg.StringField(); val != "manual_before" {
		t.Errorf("Expected 'manual_before' got '%s'", val)
	}
}

// TestAutoParse_ManualAfter ensures calling flag.Parse() after Init doesn't break anything.
func TestAutoParse_ManualAfter(t *testing.T) {
	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	os.Args = []string{"cmd", "--string_field=will_be_overridden"}
	flag.String("string_field", "defaultVal", "test field")

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field" config:"string_field"`
	}

	cfg := &TestConfig{
		StringField: func() string { return "default" },
	}

	// Do the init (this will parse flags)
	cfg.Init(cfg)

	// Manually parse again, changing the arguments
	os.Args = []string{"cmd", "--string_field=manual_after"}
	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		t.Errorf("Unexpected error on manual parse: %v", err)
	}

	// The "StringField" won't automatically pick up changes from a second parse
	// because it was already replaced with the reflect.MakeFunc() returning the old value.
	// For demonstration: we only confirm that re-calling parse does not cause error.
	// If we wanted to re-map the new command line value, we would have to orchestrate it.

	if val := cfg.StringField(); val != "will_be_overridden" {
		t.Errorf("Expected 'will_be_overridden', got '%s'", val)
	}
	// This test simply ensures that re-parsing doesn't crash or cause undesired behavior.
}

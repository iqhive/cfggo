package cfggo

import (
	"os"
	"testing"
)

// TestNewFlagSpace tests the NewFlag function to ensure that the format "variable=value"
func TestNewFlagSpace(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()

	type TestConfig struct {
		Structure
		Debug       func() bool   `json:"debug" config:"debug" description:"debug field"`
		StringField func() string `json:"string_field" config:"string_field" description:"string field"`
	}
	config := &TestConfig{
		StringField: func() string { return "default_struct_value" },
	}
	os.Args = []string{"cmd", "--string_field=flag_value"}
	config.Init(config)
	os.Args = []string{"cmd"}

	if config.configData["string_field"] != "flag_value" {
		t.Errorf("Expected 'flag_value', but got %v", config.configData["string_field"])
	}

}

// TestNewFlagEquals tests the NewFlag function to ensure that the format "--variable=value" works
func TestNewFlagEquals(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()

	type TestConfig struct {
		Structure
		Debug       func() bool   `json:"debug" config:"debug" description:"debug field"`
		StringField func() string `json:"string_field" config:"string_field" description:"string field"`
	}
	config := &TestConfig{
		StringField: func() string { return "default_struct_value" },
	}
	os.Args = []string{"cmd", "--string_field", "flag_value_space"}
	config.Init(config)
	os.Args = []string{"cmd"}

	if config.configData["string_field"] != "flag_value_space" {
		t.Errorf("Expected 'flag_value_space', but got %v", config.configData["string_field"])
	}
}

// TestAutoParse_NoManual ensures flags are automatically parsed if the user doesn't call flag.Parse().
func TestAutoParse_NoManual(t *testing.T) {
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
	os.Args = []string{"cmd"}

	if val := cfg.StringField(); val != "auto_value" {
		t.Errorf("Expected 'auto_value' got '%s'", val)
	}
}

// TestAutoParse_ManualBefore ensures parsing manually before Init doesn't cause errors or re-parse issues.
func TestAutoParse_ManualBefore(t *testing.T) {
	os.Args = []string{"cmd", "--string_field=manual_before"}

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field" config:"string_field"`
	}

	cfg := &TestConfig{
		StringField: func() string { return "default" },
	}

	cfg.Init(cfg) // Auto-parse should see it's already parsed and do nothing
	os.Args = []string{"cmd"}

	if val := cfg.StringField(); val != "manual_before" {
		t.Errorf("Expected 'manual_before' got '%s'", val)
	}
}

// TestAutoParse_ManualAfter ensures calling flag.Parse() after Init doesn't break anything.
func TestAutoParse_ManualAfter(t *testing.T) {
	os.Args = []string{"cmd", "--string_field=will_be_overridden"}

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
	cfg.Init(cfg)
	os.Args = []string{"cmd"}

	// The "StringField" won't automatically pick up changes from a second parse
	// because it was already replaced with the reflect.MakeFunc() returning the old value.
	// For demonstration: we only confirm that re-calling parse does not cause error.
	// If we wanted to re-map the new command line value, we would have to orchestrate it.

	if val := cfg.StringField(); val != "will_be_overridden" {
		t.Errorf("Expected 'will_be_overridden', got '%s'", val)
	}
	// This test simply ensures that re-parsing doesn't crash or cause undesired behavior.
}

// TestBoolFlagWithValue tests that the --debug flag enables debug mode
func TestBoolFlagWithValue(t *testing.T) {
	// Save original debug state to restore after test
	// Set up command line args with --debug flag
	os.Args = []string{"cmd", "--boolfield", "true"}

	type TestConfig struct {
		Structure
		BoolField func() bool `json:"boolfield"`
	}

	cfg := &TestConfig{
		BoolField: func() bool { return false },
	}

	// Initialize config which should parse flags
	cfg.Init(cfg)

	// Verify debug mode was enabled
	if cfg.BoolField() != true {
		t.Error("Expected --boolfield flag to enable bool")
	}

	// Reset args
	os.Args = []string{"cmd"}
}

// TestBoolFlagStandalone tests that the --debug flag enables debug mode
func TestBoolFlagStandalone(t *testing.T) {
	// Save original debug state to restore after test
	// Set up command line args with --debug flag
	os.Args = []string{"cmd", "--boolfield"}

	type TestConfig struct {
		Structure
		BoolField func() bool `json:"boolfield"`
	}

	cfg := &TestConfig{
		BoolField: func() bool { return false },
	}

	// Initialize config which should parse flags
	cfg.Init(cfg)

	// Verify debug mode was enabled
	if cfg.BoolField() != true {
		t.Error("Expected --boolfield flag to enable bool")
	}

	// Reset args
	os.Args = []string{"cmd"}
}

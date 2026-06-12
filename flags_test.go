package cfggo

import (
	"flag"
	"os"
	"testing"
)

// TestNewFlagSpace tests the NewFlag function to ensure that the format "variable=value"
func TestNewFlagSpace(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()

	type TestConfig struct {
		Structure
		Debug       func() bool   `json:"debug" cfggo:"debug" description:"debug field"`
		StringField func() string `json:"string_field" cfggo:"string_field" description:"string field"`
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
		Debug       func() bool   `json:"debug" cfggo:"debug" description:"debug field"`
		StringField func() string `json:"string_field" cfggo:"string_field" description:"string field"`
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
		StringField func() string `json:"string_field" cfggo:"string_field"`
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
		StringField func() string `json:"string_field" cfggo:"string_field"`
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
		StringField func() string `json:"string_field" cfggo:"string_field"`
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

// TestBoolFlagValueDoesNotSwallowLaterFlags verifies that the "--bool value"
// form does not stop flag parsing and drop subsequent flags (regression test:
// "--boolval true --stringval x" must still set stringval).
func TestBoolFlagValueDoesNotSwallowLaterFlags(t *testing.T) {
	os.Args = []string{"cmd", "--boolfield", "true", "--string_field", "stringstring"}

	type TestConfig struct {
		Structure
		BoolField   func() bool   `json:"boolfield"`
		StringField func() string `json:"string_field"`
	}

	cfg := &TestConfig{
		BoolField:   func() bool { return false },
		StringField: func() string { return "default" },
	}

	cfg.Init(cfg)
	os.Args = []string{"cmd"}

	if !cfg.BoolField() {
		t.Error("Expected --boolfield to be set to true")
	}
	if got := cfg.StringField(); got != "stringstring" {
		t.Errorf("Expected string_field 'stringstring', got %q", got)
	}
}

// TestBoolFlagEquals verifies the --flag=value form sets the bool explicitly.
func TestBoolFlagEquals(t *testing.T) {
	type TestConfig struct {
		Structure
		BoolField func() bool `json:"boolfield"`
	}

	for _, tt := range []struct {
		arg  string
		want bool
	}{
		{"--boolfield=true", true},
		{"--boolfield=false", false},
	} {
		os.Args = []string{"cmd", tt.arg}
		cfg := &TestConfig{BoolField: func() bool { return false }}
		cfg.Init(cfg)
		os.Args = []string{"cmd"}

		if got := cfg.BoolField(); got != tt.want {
			t.Errorf("arg %s: BoolField() = %v, want %v", tt.arg, got, tt.want)
		}
	}
}

// TestIgnoreUnknownVars verifies that, with WithIgnoreUnknownVars, an
// unrecognized flag does not terminate the process and known flags are still
// parsed.
func TestIgnoreUnknownVars(t *testing.T) {
	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field"`
	}

	for _, args := range [][]string{
		{"cmd", "--unknownflag=x", "--string_field=keep"},
		{"cmd", "--unknownflag", "value", "--string_field=keep"},
		{"cmd", "--string_field=keep", "--unknownflag=x"},
	} {
		os.Args = args
		cfg := &TestConfig{StringField: func() string { return "default" }}
		cfg.Init(cfg, WithIgnoreUnknownVars())
		os.Args = []string{"cmd"}

		if got := cfg.StringField(); got != "keep" {
			t.Errorf("args %v: StringField() = %q, want keep", args, got)
		}
	}
}

// TestWithStandardFlags verifies that cfggo can register on flag.CommandLine and
// let the host own the single flag.Parse() call, resolving cfggo's flags
// alongside another library's flag (acceptance criteria #1 and #2).
func TestWithStandardFlags(t *testing.T) {
	oldArgs := os.Args
	oldCmdLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCmdLine
	}()

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	// A flag registered by some other library/package.
	otherFlag := flag.String("someotherflag", "", "owned by another package")

	os.Args = []string{"cmd", "--string_field=std_value", "--boolfield=true", "--someotherflag=ok"}

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field"`
		BoolField   func() bool   `json:"boolfield"`
	}
	cfg := &TestConfig{
		StringField: func() string { return "default" },
		BoolField:   func() bool { return false },
	}
	cfg.Init(cfg, WithStandardFlags())

	// The host performs the single canonical parse.
	flag.Parse()

	if got := cfg.StringField(); got != "std_value" {
		t.Errorf("StringField() = %q, want std_value", got)
	}
	if !cfg.BoolField() {
		t.Error("BoolField() = false, want true")
	}
	if *otherFlag != "ok" {
		t.Errorf("someotherflag = %q, want ok", *otherFlag)
	}
}

// TestWithFlagSetSkipsAutoParse verifies that when an external flag set is
// supplied, cfggo does not parse it during Init (the host owns Parse).
func TestWithFlagSetSkipsAutoParse(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = []string{"cmd", "--string_field=later"}

	type TestConfig struct {
		Structure
		StringField func() string `json:"string_field"`
	}
	cfg := &TestConfig{StringField: func() string { return "default" }}
	cfg.Init(cfg, WithFlagSet(fs))

	if fs.Parsed() {
		t.Fatal("external flag set should not be parsed during Init")
	}
	if got := cfg.StringField(); got != "default" {
		t.Errorf("before parse: StringField() = %q, want default", got)
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := cfg.StringField(); got != "later" {
		t.Errorf("after parse: StringField() = %q, want later", got)
	}
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

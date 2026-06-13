package cfggo

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestJSONLoadingTagHierarchy(t *testing.T) {
	// Save original command line arguments and restore them after the test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Use a clean set of arguments for this test
	os.Args = []string{"test"}

	// Create a temporary directory for test files
	testDir := "tests"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	type TagTestStruct struct {
		Structure
		CfgTag   func() string `cfggo:"cfg_key" json:"json_key"`
		JsonTag  func() string `json:"json_only_key"`
		BothTags func() int    `cfggo:"cfg_preferred" json:"json_secondary"`
		NoTags   func() float64
		Nested   struct {
			InnerField func() time.Duration `cfggo:"inner_cfg_key"`
		} `cfggo:"nested"`
	}

	// Create temporary test file
	tempFileName := filepath.Join(testDir, "tag_hierarchy_test.json")
	jsonData := []byte(`{
        "cfg_key": "cfg_value",
        "json_only_key": "json_value",
        "cfg_preferred": 42,
        "NoTags": 3.14,
        "nested": {
            "inner_cfg_key": "1h30m"
        }
    }`)
	if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
	defer os.Remove(tempFileName)

	// Initialize config with a custom FlagSet to avoid global flag conflicts
	config := &TagTestStruct{
		CfgTag:   DefaultValue("default_cfg"),
		JsonTag:  DefaultValue("default_json"),
		BothTags: DefaultValue(0),
		NoTags:   DefaultValue(0.0),
	}

	// Use WithFlagSet option to provide a dedicated FlagSet for this test
	testFlagSet := flag.NewFlagSet("test_hierarchy", flag.ContinueOnError)
	_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

	// Verify values
	expected := map[string]interface{}{
		"cfg_key":              "cfg_value",
		"json_only_key":        "json_value",
		"cfg_preferred":        42,
		"NoTags":               3.14,
		"nested.inner_cfg_key": 90 * time.Minute,
	}

	for key, want := range expected {
		got := config.configData[key]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("configData[%q] = %v (%T), want %v (%T)",
				key, got, got, want, want)
		}
	}
}

func TestJSONLoadingEdgeCases(t *testing.T) {
	t.Run("hyphen_ignore", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type HyphenStruct struct {
			Structure
			IgnoredField func() string `cfggo:"-"`
		}

		// Create temporary test file
		tempFileName := filepath.Join(testDir, "hyphen_test.json")
		jsonData := []byte(`{"IgnoredField": "should_be_ignored"}`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config
		config := &HyphenStruct{
			IgnoredField: DefaultValue("default"),
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_hyphen", flag.ContinueOnError)
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Verify the field was ignored by checking if it exists in configData
		if _, exists := config.configData["IgnoredField"]; exists {
			t.Error("IgnoredField should not be present in configData")
		}
	})

	t.Run("complex_nesting", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type ComplexStruct struct {
			Structure
			Database struct {
				Host func() string `cfggo:"db_host"`
				Port func() int    `json:"db_port"`
			} `cfggo:"database"`
		}

		// Create temporary test file
		tempFileName := filepath.Join(testDir, "complex_nesting_test.json")
		jsonData := []byte(`{
            "database": {
                "db_host": "localhost",
                "db_port": 5432
            }
        }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config
		config := &ComplexStruct{
			Database: struct {
				Host func() string `cfggo:"db_host"`
				Port func() int    `json:"db_port"`
			}{
				Host: DefaultValue("default_host"),
				Port: DefaultValue(0),
			},
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_complex", flag.ContinueOnError)
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Verify using configData
		expected := map[string]interface{}{
			"database.db_host": "localhost",
			"database.db_port": 5432,
		}

		for key, want := range expected {
			got := config.configData[key]
			if !reflect.DeepEqual(got, want) {
				t.Errorf("configData[%q] = %v (%T), want %v (%T)", key, got, got, want, want)
			}
		}
	})

	t.Run("type_conversions", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type TypeTestStruct struct {
			Structure
			IntToFloat   func() float64 `cfggo:"int_field"`
			StringToBool func() bool    `cfggo:"bool_field"`
		}

		// Create temporary test file
		tempFileName := filepath.Join(testDir, "type_conversion_test.json")
		jsonData := []byte(`{
            "int_field": 42,
            "bool_field": "true"
        }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config
		config := &TypeTestStruct{
			IntToFloat:   DefaultValue(0.0),
			StringToBool: DefaultValue(false),
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_types", flag.ContinueOnError)
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Verify type conversions using configData
		if got, want := config.configData["int_field"], 42.0; got != want {
			t.Errorf("IntToFloat = %v (%T), want %v (%T)", got, got, want, want)
		}
		if got, want := config.configData["bool_field"], true; got != want {
			t.Errorf("StringToBool = %v (%T), want %v (%T)", got, got, want, want)
		}
	})

	t.Run("duration_formats", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type DurationTestStruct struct {
			Structure
			Seconds      func() time.Duration `cfggo:"seconds"`      // Test "1s" format
			Milliseconds func() time.Duration `cfggo:"milliseconds"` // Test "10ms" format
			Minutes      func() time.Duration `cfggo:"minutes"`      // Test "2m" format
			Complex      func() time.Duration `cfggo:"complex"`      // Test "1h2m3s" format
			Numerical    func() time.Duration `cfggo:"numerical"`    // Test numeric value (nanoseconds)
		}

		// Create temporary test file
		tempFileName := filepath.Join(testDir, "duration_test.json")
		jsonData := []byte(`{
            "seconds": "1s",
            "milliseconds": "10ms",
            "minutes": "2m",
            "complex": "1h2m3s",
            "numerical": 5000000000
        }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config with zero duration defaults
		config := &DurationTestStruct{
			Seconds:      DefaultValue(time.Duration(0)),
			Milliseconds: DefaultValue(time.Duration(0)),
			Minutes:      DefaultValue(time.Duration(0)),
			Complex:      DefaultValue(time.Duration(0)),
			Numerical:    DefaultValue(time.Duration(0)),
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_durations", flag.ContinueOnError)
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Expected durations
		expected := map[string]time.Duration{
			"seconds":      1 * time.Second,
			"milliseconds": 10 * time.Millisecond,
			"minutes":      2 * time.Minute,
			"complex":      1*time.Hour + 2*time.Minute + 3*time.Second,
			"numerical":    5 * time.Second, // 5 billion nanoseconds = 5 seconds
		}

		// Verify duration parsing
		for key, want := range expected {
			got, ok := config.configData[key].(time.Duration)
			if !ok {
				t.Errorf("configData[%q] is not a time.Duration, got %T", key, config.configData[key])
				continue
			}
			if got != want {
				t.Errorf("configData[%q] = %v, want %v", key, got, want)
			}
		}

		// Also verify that the functions return the correct values
		if got := config.Seconds(); got != expected["seconds"] {
			t.Errorf("Seconds() = %v, want %v", got, expected["seconds"])
		}
		if got := config.Milliseconds(); got != expected["milliseconds"] {
			t.Errorf("Milliseconds() = %v, want %v", got, expected["milliseconds"])
		}
		if got := config.Minutes(); got != expected["minutes"] {
			t.Errorf("Minutes() = %v, want %v", got, expected["minutes"])
		}
		if got := config.Complex(); got != expected["complex"] {
			t.Errorf("Complex() = %v, want %v", got, expected["complex"])
		}
		if got := config.Numerical(); got != expected["numerical"] {
			t.Errorf("Numerical() = %v, want %v", got, expected["numerical"])
		}
	})
}

func TestJSONLoadingErrorHandling(t *testing.T) {
	t.Run("invalid_json", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type InvalidJSONStruct struct {
			Structure
			Field func() string `cfggo:"test_field"`
		}

		// Create temporary test file with invalid JSON
		tempFileName := filepath.Join(testDir, "invalid_json_test.json")
		jsonData := []byte(`{ "test_field": "value" `) // Missing closing brace
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config
		config := &InvalidJSONStruct{
			Field: DefaultValue("default"),
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_invalid_json", flag.ContinueOnError)

		// Should not panic on invalid JSON
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Verify the default value was retained
		if got := config.Field(); got != "default" {
			t.Errorf("Field = %q, want %q", got, "default")
		}
	})

	t.Run("unexpected_type", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Use a clean set of arguments for this test
		os.Args = []string{"test"}

		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		type TypeMismatchStruct struct {
			Structure
			Number func() int `cfggo:"number_field"`
		}

		// Create temporary test file with type mismatch
		tempFileName := filepath.Join(testDir, "type_mismatch_test.json")
		jsonData := []byte(`{ "number_field": "not_an_int" }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		// Initialize config
		config := &TypeMismatchStruct{
			Number: DefaultValue(0),
		}

		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_type_mismatch", flag.ContinueOnError)

		// Should not panic on type mismatch
		_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		// Verify the default value was retained
		if got := config.Number(); got != 0 {
			t.Errorf("Number = %d, want %d", got, 0)
		}
	})
}

func TestAutoSaveDisabledByDefault(t *testing.T) {
	// Save original command line arguments and restore them after the test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Use a clean set of arguments for this test
	os.Args = []string{"test"}

	type SimpleConfig struct {
		Structure
		Field func() string `cfggo:"field"`
	}

	// Initialize config
	config := &SimpleConfig{
		Field: DefaultValue("default_value"),
	}

	// Use WithFlagSet option to provide a dedicated FlagSet for this test
	testFlagSet := flag.NewFlagSet("test_auto_save", flag.ContinueOnError)
	_ = config.Init(config, WithFlagSet(testFlagSet))

	// Verify autoSave is false by default
	if config.autoSave {
		t.Error("autoSave should be false by default")
	}

	// Without WithAutoSave there should be no auto-save context wired up.
	if config.autoSaveCtx != nil {
		t.Error("autoSaveCtx should be nil when autoSave is disabled")
	}
}

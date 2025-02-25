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
	
	// Create a new FlagSet for this test to avoid conflicts
	originalFlagSet := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = originalFlagSet }()
	
	// Create a temporary directory for test files
	testDir := "tests"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	
	type TagTestStruct struct {
		Structure
		CfgTag   func() string `cfg:"cfg_key" json:"json_key"`
		JsonTag  func() string `json:"json_only_key"`
		BothTags func() int    `cfg:"cfg_preferred" json:"json_secondary"`
		NoTags   func() float64
		Nested   struct {
			InnerField func() time.Duration `cfg:"inner_cfg_key"`
		} `cfg:"nested"`
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
	config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

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
		
		// Create a new FlagSet for this test to avoid conflicts
		originalFlagSet := flag.CommandLine
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		defer func() { flag.CommandLine = originalFlagSet }()
		
		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		
		type HyphenStruct struct {
			Structure
			IgnoredField func() string `cfg:"-"`
		}

		tempFileName := filepath.Join(testDir, "hyphen_test.json")
		jsonData := []byte(`{"IgnoredField": "should_be_ignored"}`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		config := &HyphenStruct{
			IgnoredField: DefaultValue("default"),
		}
		
		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_hyphen", flag.ContinueOnError)
		config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

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
		
		// Create a new FlagSet for this test to avoid conflicts
		originalFlagSet := flag.CommandLine
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		defer func() { flag.CommandLine = originalFlagSet }()
		
		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		
		type ComplexStruct struct {
			Structure
			Database struct {
				Host func() string `cfg:"db_host"`
				Port func() int    `json:"db_port"`
			} `cfg:"database"`
		}

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

		config := &ComplexStruct{
			Database: struct {
				Host func() string `cfg:"db_host"`
				Port func() int    `json:"db_port"`
			}{
				Host: DefaultValue("default_host"),
				Port: DefaultValue(0),
			},
		}
		
		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_complex", flag.ContinueOnError)
		config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

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
		
		// Create a new FlagSet for this test to avoid conflicts
		originalFlagSet := flag.CommandLine
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		defer func() { flag.CommandLine = originalFlagSet }()
		
		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		
		type TypeTestStruct struct {
			Structure
			IntToFloat   func() float64 `cfg:"int_field"`
			StringToBool func() bool    `cfg:"bool_field"`
		}

		tempFileName := filepath.Join(testDir, "type_conversion_test.json")
		jsonData := []byte(`{
            "int_field": 42,
            "bool_field": "true"
        }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		config := &TypeTestStruct{
			IntToFloat:   DefaultValue(0.0),
			StringToBool: DefaultValue(false),
		}
		
		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_types", flag.ContinueOnError)
		config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

		if got, want := config.configData["int_field"], 42.0; got != want {
			t.Errorf("IntToFloat = %v (%T), want %v (%T)", got, got, want, want)
		}
		if got, want := config.configData["bool_field"], true; got != want {
			t.Errorf("StringToBool = %v (%T), want %v (%T)", got, got, want, want)
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
		
		// Create a new FlagSet for this test to avoid conflicts
		originalFlagSet := flag.CommandLine
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		defer func() { flag.CommandLine = originalFlagSet }()
		
		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		
		type InvalidJSONStruct struct {
			Structure
			Field func() string `cfg:"test_field"`
		}

		tempFileName := filepath.Join(testDir, "invalid_json_test.json")
		jsonData := []byte(`{ "test_field": "value" `) // Missing closing brace
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		config := &InvalidJSONStruct{
			Field: DefaultValue("default"),
		}
		
		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_invalid_json", flag.ContinueOnError)
		config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))
		// if err == nil {
		// 	t.Error("Expected error for invalid JSON, got nil")
		// }
	})

	t.Run("unexpected_type", func(t *testing.T) {
		// Save original command line arguments and restore them after the test
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()
		
		// Use a clean set of arguments for this test
		os.Args = []string{"test"}
		
		// Create a new FlagSet for this test to avoid conflicts
		originalFlagSet := flag.CommandLine
		flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
		defer func() { flag.CommandLine = originalFlagSet }()
		
		// Create a temporary directory for test files
		testDir := "tests"
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		
		type TypeMismatchStruct struct {
			Structure
			Number func() int `cfg:"number_field"`
		}

		tempFileName := filepath.Join(testDir, "type_mismatch_test.json")
		jsonData := []byte(`{ "number_field": "not_an_int" }`)
		if err := os.WriteFile(tempFileName, jsonData, 0644); err != nil {
			t.Fatalf("failed to write temp config file: %v", err)
		}
		defer os.Remove(tempFileName)

		config := &TypeMismatchStruct{
			Number: DefaultValue(0),
		}
		
		// Use WithFlagSet option to provide a dedicated FlagSet for this test
		testFlagSet := flag.NewFlagSet("test_type_mismatch", flag.ContinueOnError)
		config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))
		// if err == nil {
		// 	t.Error("Expected error for type mismatch, got nil")
		// }
	})
}

func TestAutoSaveDisabledByDefault(t *testing.T) {
	// Save original command line arguments and restore them after the test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	
	// Use a clean set of arguments for this test
	os.Args = []string{"test"}
	
	// Create a new FlagSet for this test to avoid conflicts
	originalFlagSet := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = originalFlagSet }()
	
	type SimpleConfig struct {
		Structure
		Field func() string `cfg:"field"`
	}

	config := &SimpleConfig{
		Field: DefaultValue("default_value"),
	}
	
	// Use WithFlagSet option to provide a dedicated FlagSet for this test
	testFlagSet := flag.NewFlagSet("test_auto_save", flag.ContinueOnError)
	config.Init(config, WithFlagSet(testFlagSet))

	// Verify autoSave is false by default
	if config.autoSave {
		t.Error("autoSave should be false by default")
	}

	// Verify configsToSave doesn't contain our config
	found := false
	for _, c := range configsToSave {
		if c == &config.Structure {
			found = true
			break
		}
	}
	if found {
		t.Error("config should not be in configsToSave when autoSave is disabled")
	}
}

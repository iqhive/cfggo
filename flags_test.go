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

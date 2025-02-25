package cfggo

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestReload(t *testing.T) {
	// Save original command line arguments and restore them after the test
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Create a dedicated FlagSet for this test to avoid conflicts
	testFlagSet := flag.NewFlagSet("reload-test", flag.ContinueOnError)

	// Create a temporary directory for test files
	testDir := "tests"
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	type ReloadConfig struct {
		Structure
		Name    func() string `cfg:"name"`
		Version func() int    `cfg:"version"`
	}

	// Create initial config file
	tempFileName := filepath.Join(testDir, "reload_test.json")
	initialJSON := []byte(`{
		"name": "initial",
		"version": 1
	}`)
	if err := os.WriteFile(tempFileName, initialJSON, 0644); err != nil {
		t.Fatalf("failed to write initial config file: %v", err)
	}
	defer os.Remove(tempFileName)

	// Initialize config with dedicated FlagSet
	config := &ReloadConfig{
		Name:    DefaultValue("default"),
		Version: DefaultValue(0),
	}
	config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

	// Verify initial values
	if got, want := config.Name(), "initial"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := config.Version(), 1; got != want {
		t.Errorf("Version = %d, want %d", got, want)
	}

	// Update config file
	updatedJSON := []byte(`{
		"name": "updated",
		"version": 2
	}`)
	if err := os.WriteFile(tempFileName, updatedJSON, 0644); err != nil {
		t.Fatalf("failed to write updated config file: %v", err)
	}

	// Reload config
	if err := config.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}

	// Verify updated values
	if got, want := config.Name(), "updated"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := config.Version(), 2; got != want {
		t.Errorf("Version = %d, want %d", got, want)
	}
}

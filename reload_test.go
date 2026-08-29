package cfggo

import (
	"encoding/json"
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
		Name    func() string `cfggo:"name"`
		Version func() int    `cfggo:"version"`
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
	_ = config.Init(config, WithFileConfig(tempFileName), WithFlagSet(testFlagSet))

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

// TestReloadWithMutableDefault verifies that reload (specifically
// resetToDefaultsLocked) correctly resets mutable default values (maps) after
// a file-source override.
func TestReloadWithMutableDefault(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"test"}

	type MutableConfig struct {
		Structure
		Tags func() map[string]string `cfggo:"tags"`
	}

	cfg := &MutableConfig{
		Tags: DefaultValue(map[string]string{"key": "prod"}),
	}
	handler := &memHandler{data: json.RawMessage(`{"tags":{"key":"staging"}}`)}
	fs := flag.NewFlagSet("mutable-test", flag.ContinueOnError)
	if err := cfg.Init(cfg, WithFlagSet(fs), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if tags := cfg.Tags(); tags["key"] != "staging" {
		t.Fatalf("Init: tags[key]=%q, want \"staging\"", tags["key"])
	}

	// Reload must reload from handler (which provides "staging" again)
	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if tags := cfg.Tags(); tags["key"] != "staging" {
		t.Fatalf("After reload: tags[key]=%q, want \"staging\"", tags["key"])
	}

	// Mutate the value returned by the handler v Instead of getting it
	tags := cfg.Tags()
	tags["key"] = "mutated"

	// Reload again — should revert to "staging" from handler, not the mutation
	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("Second Reload: %v", err)
	}
	if tags := cfg.Tags(); tags["key"] != "staging" {
		t.Fatalf("After mutation+reload: tags[key]=%q, want \"staging\"; defaultData may have been corrupted", tags["key"])
	}
}

// TestReloadWithMutableSliceDefault verifies the same for slices.
func TestReloadWithMutableSliceDefault(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"test"}

	type SliceConfig struct {
		Structure
		Items func() []string `cfggo:"items"`
	}

	cfg := &SliceConfig{
		Items: DefaultValue([]string{"a", "b"}),
	}
	handler := &memHandler{data: json.RawMessage(`{"items":["c","d"]}`)}
	fs := flag.NewFlagSet("slice-test", flag.ContinueOnError)
	if err := cfg.Init(cfg, WithFlagSet(fs), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if it := cfg.Items(); it[0] != "c" || it[1] != "d" {
		t.Fatalf("Init: items=%v, want [c d]", it)
	}

	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if it := cfg.Items(); it[0] != "c" || it[1] != "d" {
		t.Fatalf("After reload: items=%v, want [c d]", it)
	}

	// Mutate via the accessor
	items := cfg.Items()
	items[0] = "x"

	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("Second Reload: %v", err)
	}
	if it := cfg.Items(); it[0] != "c" || it[1] != "d" {
		t.Fatalf("After mutation+reload: items=%v, want [c d]; defaults may be corrupted", it)
	}
}

package cfggo

import (
	"errors"
	"flag"
	"os"
	"testing"
)

func TestValidation(t *testing.T) {
	// Save original command line arguments and restore them after the test
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	type ValidationConfig struct {
		Structure
		Age    func() int    `cfggo:"age"`
		Email  func() string `cfggo:"email"`
		Active func() bool   `cfggo:"active"`
	}

	t.Run("valid_configuration", func(t *testing.T) {
		// Create a dedicated FlagSet for this test to avoid conflicts
		testFlagSet := flag.NewFlagSet("validation-test-valid", flag.ContinueOnError)
		// Set os.Args to avoid flag parsing errors
		os.Args = []string{"test"}

		config := &ValidationConfig{
			Age:    DefaultValue(25),
			Email:  DefaultValue("test@example.com"),
			Active: DefaultValue(true),
		}

		// Add validators
		ageValidator := func(value interface{}) error {
			age, ok := value.(int)
			if !ok {
				return errors.New("age must be an integer")
			}
			if age < 0 || age > 120 {
				return errors.New("age must be between 0 and 120")
			}
			return nil
		}

		// Initialize the config with the validator
		config.Init(config, WithFlagSet(testFlagSet))

		// Register validator after initialization
		config.RegisterValidator("age", ageValidator)

		// Validate should pass
		if err := config.Validate(); err != nil {
			t.Errorf("Validation failed: %v", err)
		}

		// ValidateKey should pass
		if err := config.ValidateKey("age"); err != nil {
			t.Errorf("Validation of key 'age' failed: %v", err)
		}
	})

	t.Run("invalid_configuration", func(t *testing.T) {
		// Create a dedicated FlagSet for this test to avoid conflicts
		testFlagSet := flag.NewFlagSet("validation-test-invalid", flag.ContinueOnError)
		// Set os.Args to avoid flag parsing errors
		os.Args = []string{"test"}

		config := &ValidationConfig{
			Age:    DefaultValue(150), // Invalid age
			Email:  DefaultValue("test@example.com"),
			Active: DefaultValue(true),
		}

		// Initialize the config first
		config.Init(config, WithFlagSet(testFlagSet))

		// Add validators after initialization
		ageValidator := func(value interface{}) error {
			age, ok := value.(int)
			if !ok {
				return errors.New("age must be an integer")
			}
			if age < 0 || age > 120 {
				return errors.New("age must be between 0 and 120")
			}
			return nil
		}

		// Register validator after initialization
		config.RegisterValidator("age", ageValidator)

		// Validate should fail
		if err := config.Validate(); err == nil {
			t.Error("Validation should have failed for invalid age")
		}

		// ValidateKey should fail
		if err := config.ValidateKey("age"); err == nil {
			t.Error("Validation of key 'age' should have failed")
		}
	})
}

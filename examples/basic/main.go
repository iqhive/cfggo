package main

import (
	"fmt"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/cfglogger"
)

// MyConfig represents a basic configuration struct
type MyConfig struct {
	cfggo.Structure

	AppName func() string `cfggo:"app_name" default:"my-app" help:"Name of the application"`
	Port    func() int    `cfggo:"port" default:"8080" help:"Port to listen on"`
	Debug   func() bool   `cfggo:"debug" default:"false" help:"Enable debug mode"`
	Timeout func() string `cfggo:"timeout" default:"30s" help:"Request timeout"`
}

func main() {
	// Example 1: Basic usage with global logger (backwards compatible)
	fmt.Println("=== Example 1: Basic usage (backwards compatible) ===")

	config1 := &MyConfig{}
	config1.Init(config1)

	fmt.Printf("App Name: %s\n", config1.AppName())
	fmt.Printf("Port: %d\n", config1.Port())
	fmt.Printf("Debug: %t\n", config1.Debug())
	fmt.Printf("Timeout: %s\n", config1.Timeout())

	// Example 2: Using custom logger for this instance
	fmt.Println("\n=== Example 2: Custom logger per instance ===")

	// Create a custom logger
	customLogger := cfglogger.NewDefaultLogger()

	config2 := &MyConfig{}
	config2.Init(config2, cfggo.WithLogger(customLogger))

	// Test error handling with custom logger
	err := config2.Set("port", "invalid-port")
	if err != nil {
		fmt.Printf("Error (with custom logger): %v\n", err)
	}

	// Example 3: Using WrapErrorWithLogging to demonstrate logging
	fmt.Println("\n=== Example 3: Error wrapping with logging ===")

	config3 := &MyConfig{}
	config3.Init(config3)

	// This will trigger error logging
	err = config3.WrapErrorWithLogging(nil, 500, "This is a test error message")
	if err != nil {
		fmt.Printf("Wrapped error: %v\n", err)
	}

	fmt.Println("\n=== Example 4: Multiple instances with different loggers ===")

	// Create multiple configurations with different loggers
	config4a := &MyConfig{}
	config4a.Init(config4a, cfggo.WithName("service-a"))

	config4b := &MyConfig{}
	config4b.Init(config4b, cfggo.WithName("service-b"), cfggo.WithLogger(&cfglogger.NoopLogger{}))

	// Set different values for each
	config4a.Set("app_name", "service-a")
	config4a.Set("port", 9001)

	config4b.Set("app_name", "service-b")
	config4b.Set("port", 9002)

	fmt.Printf("Service A - Name: %s, Port: %d\n", config4a.AppName(), config4a.Port())
	fmt.Printf("Service B - Name: %s, Port: %d\n", config4b.AppName(), config4b.Port())
}

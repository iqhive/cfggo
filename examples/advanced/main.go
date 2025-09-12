package main

import (
	"fmt"
	"log"

	"github.com/iqhive/cfggo"
)

// CustomLogger demonstrates a custom logger implementation
type CustomLogger struct {
	prefix string
}

func NewCustomLogger(prefix string) *CustomLogger {
	return &CustomLogger{prefix: prefix}
}

func (cl *CustomLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[%s DEBUG] "+msg, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Debugf(format string, args ...interface{}) {
	log.Printf("[%s DEBUG] "+format, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Info(msg string, args ...interface{}) {
	log.Printf("[%s INFO] "+msg, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Infof(format string, args ...interface{}) {
	log.Printf("[%s INFO] "+format, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[%s WARN] "+msg, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Warnf(format string, args ...interface{}) {
	log.Printf("[%s WARN] "+format, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Error(msg string, args ...interface{}) {
	log.Printf("[%s ERROR] "+msg, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Errorf(format string, args ...interface{}) {
	log.Printf("[%s ERROR] "+format, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Fatal(msg string, args ...interface{}) {
	log.Printf("[%s FATAL] "+msg, append([]interface{}{cl.prefix}, args...)...)
}

func (cl *CustomLogger) Fatalf(format string, args ...interface{}) {
	log.Printf("[%s FATAL] "+format, append([]interface{}{cl.prefix}, args...)...)
}

// CustomErrorWrapper demonstrates a custom error wrapper
func CustomErrorWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	if msg == "" {
		return fmt.Errorf("[CODE:%d] %v", errorcode, err)
	}
	return fmt.Errorf("[CODE:%d] "+msg, append([]interface{}{errorcode}, args...)...)
}

// AdvancedConfig demonstrates advanced configuration with custom components
type AdvancedConfig struct {
	cfggo.Structure

	ServiceName   func() string `cfggo:"service_name" default:"advanced-service" help:"Name of the service"`
	LogLevel      func() string `cfggo:"log_level" default:"info" help:"Logging level"`
	MaxRetries    func() int    `cfggo:"max_retries" default:"3" help:"Maximum number of retries"`
	EnableMetrics func() bool   `cfggo:"enable_metrics" default:"true" help:"Enable metrics collection"`
}

func main() {
	fmt.Println("=== Advanced Error Handling and Logging Example ===")

	// Example 1: Custom logger with different prefixes for different services
	fmt.Println("\n--- Example 1: Multiple services with custom loggers ---")

	serviceALogger := NewCustomLogger("SERVICE-A")
	serviceBLogger := NewCustomLogger("SERVICE-B")

	configA := &AdvancedConfig{}
	configA.Init(configA,
		cfggo.WithName("service-a"),
		cfggo.WithLogger(serviceALogger),
	)

	configB := &AdvancedConfig{}
	configB.Init(configB,
		cfggo.WithName("service-b"),
		cfggo.WithLogger(serviceBLogger),
		cfggo.WithErrorWrapper(CustomErrorWrapper),
	)

	// Demonstrate logging through error wrapper
	fmt.Println("\nTesting error handling:")

	// Service A will use default error wrapper with custom logger
	err := configA.WrapErrorWithLogging(nil, 404, "Resource not found in service A")
	if err != nil {
		fmt.Printf("Service A error: %v\n", err)
	}

	// Service B will use custom error wrapper
	err = configB.WrapError(nil, 500, "Internal error in service B")
	if err != nil {
		fmt.Printf("Service B error: %v\n", err)
	}

	// Example 2: Type conversion with instance-specific error handling
	fmt.Println("\n--- Example 2: Type conversion errors ---")

	err = configA.Set("max_retries", "not-a-number")
	if err != nil {
		fmt.Printf("Type conversion error in Service A: %v\n", err)
	}

	err = configB.Set("max_retries", "also-not-a-number")
	if err != nil {
		fmt.Printf("Type conversion error in Service B: %v\n", err)
	}

	// Example 3: Successful configuration
	fmt.Println("\n--- Example 3: Valid configuration ---")

	configA.Set("service_name", "analytics-service")
	configA.Set("max_retries", 5)

	configB.Set("service_name", "notification-service")
	configB.Set("log_level", "debug")

	fmt.Printf("Service A: %s, Retries: %d\n", configA.ServiceName(), configA.MaxRetries())
	fmt.Printf("Service B: %s, Log Level: %s\n", configB.ServiceName(), configB.LogLevel())

	// Example 4: Demonstrating backwards compatibility
	fmt.Println("\n--- Example 4: Backwards compatibility ---")

	// This should work exactly like before the refactoring
	legacyConfig := &AdvancedConfig{}
	legacyConfig.Init(legacyConfig) // No custom options, uses globals

	fmt.Printf("Legacy config service name: %s\n", legacyConfig.ServiceName())
	fmt.Printf("Legacy config uses global logger: %t\n", legacyConfig.GetLogger() == cfggo.Logger)
}

// Package main demonstrates cfggo's validation API.
package main

import (
	"fmt"
	"log"

	"github.com/iqhive/cfggo"
)

// ServerConfig holds configuration for an HTTP server with validation rules.
type ServerConfig struct {
	cfggo.Structure

	ServerPort     func() int    `cfggo:"server_port"     default:"8080"  help:"HTTP listen port"`
	AdminEmail     func() string `cfggo:"admin_email"     default:""       help:"Admin contact email"`
	LogLevel       func() string `cfggo:"log_level"       default:"info"   help:"Log verbosity"`
	MaxConnections func() int    `cfggo:"max_connections" default:"50"    help:"Maximum concurrent connections"`
	APIKey         func() string `cfggo:"api_key"         default:""       help:"32-character API key"`
}

func main() {
	cfg := &ServerConfig{}

	// --- Register validators before or during Init. ---

	// Port must be in the valid unprivileged range.
	cfg.AddValidator("server_port", cfggo.Range(1024, 65535))

	// Admin email must look like an email address.
	cfg.AddValidator("admin_email", cfggo.All(
		cfggo.Required(),
		cfggo.Email(),
	))

	// Log level must be one of the recognised strings.
	cfg.AddValidator("log_level", cfggo.OneOf("debug", "info", "warn", "error"))

	// Max connections must be at least 1.
	cfg.AddValidator("max_connections", cfggo.Range(1, 10000))

	// API key must match a 32-character alphanumeric pattern.
	cfg.AddValidator("api_key", cfggo.All(
		cfggo.Required(),
		cfggo.Regex(`^[A-Za-z0-9]{32}$`),
	))

	// Load from the bundled config.json (values override defaults).
	cfg.Init(cfg, cfggo.WithDefaultFileConfig("config.json"))

	// --- Run validation explicitly. ---
	// (Init also runs Validate, but here we show calling it manually.)
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration is invalid:\n%v", err)
	}

	fmt.Println("Configuration is valid!")
	fmt.Printf("  server_port     : %d\n", cfg.ServerPort())
	fmt.Printf("  admin_email     : %s\n", cfg.AdminEmail())
	fmt.Printf("  log_level       : %s\n", cfg.LogLevel())
	fmt.Printf("  max_connections : %d\n", cfg.MaxConnections())
	fmt.Printf("  api_key         : %s\n", cfg.APIKey())

	// --- Demonstrate per-key validation. ---
	fmt.Println()
	if err := cfg.ValidateKey("server_port"); err != nil {
		fmt.Printf("server_port invalid: %v\n", err)
	} else {
		fmt.Printf("server_port %d is valid\n", cfg.ServerPort())
	}

	// --- Show what a validation failure looks like. ---
	fmt.Println("\n-- Setting an invalid port to show failure output --")
	_ = cfg.Set("server_port", 80) // below the 1024 minimum
	if err := cfg.ValidateKey("server_port"); err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}

	// --- Custom validator via cfggo.Custom. ---
	cfg.AddValidator("max_connections", cfggo.Custom(func(v interface{}) error {
		n, ok := v.(int)
		if !ok {
			return fmt.Errorf("expected int, got %T", v)
		}
		if n%10 != 0 {
			return fmt.Errorf("max_connections must be a multiple of 10, got %d", n)
		}
		return nil
	}))
	// 100 is a multiple of 10, so validation passes.
	_ = cfg.Set("max_connections", 100)
	if err := cfg.ValidateKey("max_connections"); err != nil {
		fmt.Printf("max_connections invalid: %v\n", err)
	} else {
		fmt.Printf("max_connections %d is valid (multiple-of-10 check)\n", cfg.MaxConnections())
	}
}

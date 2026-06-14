// Package main demonstrates cfggo's startup/debugging support.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iqhive/cfggo"
)

type DebugConfig struct {
	cfggo.Structure

	Port       func() int    `cfggo:"port" default:"8080" help:"HTTP listen port"`
	LogLevel   func() string `cfggo:"log_level" default:"info" help:"Logging level"`
	AdminEmail func() string `cfggo:"admin_email" default:"ops@example.com" help:"Admin contact email"`
	APIKey     func() string `cfggo:"api_key" default:"dev-secret" secret:"true" help:"API key"`
}

func main() {
	dir, err := os.MkdirTemp("", "cfggo-debugging-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	fmt.Println("=== Blessed Startup Pattern ===")
	showStartupPattern(writeFile(dir, "startup-check.json", `{"port":9090,"stale_key":true}`))

	fmt.Println("=== Failure Modes ===")
	showInitError("bad JSON", initFromFile(writeFile(dir, "bad.json", `{`)))
	showInitError("wrong type", initFromFile(writeFile(dir, "wrong-type.json", `{"port":"not-a-number"}`)))
	showInitError("unknown key", initStrictFromFile(writeFile(dir, "unknown-key.json", `{"port":9090,"stale_key":true}`)))
	showInitError("failed validator", initInvalidValidatedConfig(writeFile(dir, "invalid-validator.json", `{"admin_email":"not-an-email"}`)))

	fmt.Println("\n=== Overrides ===")
	showEnvOverride()
	showFlagOverride()

	fmt.Println("\n=== Secret Redaction ===")
	showSecretRedaction()
}

func initFromFile(path string) error {
	cfg := &DebugConfig{}
	return cfggo.Init(cfg, cfggo.WithFileConfig(path), cfggo.WithoutFlags())
}

func showStartupPattern(path string) {
	cfg := &DebugConfig{}
	if err := cfggo.Init(cfg,
		cfggo.WithFileConfig(path),
		cfggo.WithStrictKeys(),
		cfggo.WithoutFlags(),
	); err != nil {
		fmt.Printf("load configuration: %v\n", err)
		fmt.Println("redacted report:")
		fmt.Println(cfg.Report())
	}
}

func initStrictFromFile(path string) error {
	cfg := &DebugConfig{}
	return cfggo.Init(cfg, cfggo.WithFileConfig(path), cfggo.WithStrictKeys(), cfggo.WithoutFlags())
}

func initInvalidValidatedConfig(path string) error {
	cfg := &DebugConfig{}
	cfg.AddValidator("admin_email", cfggo.Email())
	return cfggo.Init(cfg, cfggo.WithFileConfig(path), cfggo.WithoutFlags())
}

func showInitError(label string, err error) {
	if err == nil {
		fmt.Printf("%-16s: unexpectedly succeeded\n", label)
		return
	}
	fmt.Printf("%-16s: %v\n", label, err)
}

func showEnvOverride() {
	if err := os.Setenv("PORT", "9090"); err != nil {
		panic(err)
	}
	defer os.Unsetenv("PORT")

	cfg := &DebugConfig{}
	if err := cfggo.Init(cfg, cfggo.WithoutFlags()); err != nil {
		panic(err)
	}
	src, _ := cfg.Source("port")
	fmt.Printf("env override    : port=%d (from %s)\n", cfg.Port(), src)
}

func showFlagOverride() {
	cfg := &DebugConfig{}
	fs := flag.NewFlagSet("debugging", flag.ContinueOnError)
	if err := cfggo.Init(cfg, cfggo.WithFlagSet(fs)); err != nil {
		panic(err)
	}
	if err := fs.Parse([]string{"--port=7070"}); err != nil {
		panic(err)
	}
	src, _ := cfg.Source("port")
	fmt.Printf("flag override   : port=%d (from %s)\n", cfg.Port(), src)
}

func showSecretRedaction() {
	cfg := &DebugConfig{}
	if err := cfggo.Init(cfg, cfggo.WithoutFlags()); err != nil {
		panic(err)
	}
	if err := cfg.Set("api_key", "super-secret-token"); err != nil {
		panic(err)
	}
	fmt.Println(cfg.Report())
}

func writeFile(dir, name, data string) string {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		panic(err)
	}
	return path
}

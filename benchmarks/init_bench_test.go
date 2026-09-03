package benchmarks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iqhive/cfggo"
)

// Init does the bulk of cfggo's reflective work: it walks the struct, sets
// defaults from tags, installs accessor closures, loads each configured source,
// and registers flags. These benchmarks isolate that cost by struct size and by
// configuration source.

func BenchmarkInitSmall(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &smallConfig{}
		mustInit(b, cfg)
	}
}

func BenchmarkInitMedium(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &mediumConfig{}
		mustInit(b, cfg)
	}
}

func BenchmarkInitLarge(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &largeConfig{}
		mustInit(b, cfg)
	}
}

func BenchmarkInitNested(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &nestedConfig{}
		mustInit(b, cfg)
	}
}

// BenchmarkInitWithMemHandler includes loading + JSON-decoding a full config
// document through a ConfigHandler, without disk I/O.
func BenchmarkInitWithMemHandler(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &mediumConfig{}
		mustInit(b, cfg,
			cfggo.WithoutEnv(),
			cfggo.WithConfigHandler(newMemHandler(mediumJSON)),
		)
	}
}

// BenchmarkInitWithFile includes the full file-backed load path (open, read,
// unmarshal) on every iteration.
func BenchmarkInitWithFile(b *testing.B) {
	dir := b.TempDir()
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte(mediumJSON), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := &mediumConfig{}
		mustInit(b, cfg,
			cfggo.WithoutEnv(),
			cfggo.WithFileConfig(file),
		)
	}
}

// BenchmarkInitWithEnv includes the environment-variable scan/apply path. A
// subset of medium keys are exported so real values flow through the converter.
func BenchmarkInitWithEnv(b *testing.B) {
	env := map[string]string{
		"APP_NAME":  "from-env",
		"PORT":      "7000",
		"DEBUG":     "true",
		"TIMEOUT":   "90s",
		"LOG_LEVEL": "warn",
		"REGION":    "ap-south-1",
	}
	for k, v := range env {
		b.Setenv(k, v)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := &mediumConfig{}
		// Note: env is intentionally NOT skipped here.
		mustInit(b, cfg, cfggo.WithName("envbench"))
	}
}

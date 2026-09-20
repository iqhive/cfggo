package benchmarks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/internal/convert"
)

// BenchmarkInitSeed measures Init on a struct with several default-tagged
// fields (the Wave 2 seed path).
func BenchmarkInitSeed(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &smallConfig{}
		mustInit(b, cfg)
	}
}

// BenchmarkReload measures the ReloadConfig cycle on a small file-backed
// config (the Wave 2 reload path).
func BenchmarkReload(b *testing.B) {
	dir := b.TempDir()
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte(`{"name":"from-file","port":9090}`), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}

	cfg := &smallConfig{}
	mustInit(b, cfg,
		cfggo.WithoutEnv(),
		cfggo.WithFileConfig(file),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.ReloadConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetFlagSet measures GetFlagSet on an initialised config (the
// Wave 2 flag-set path).
func BenchmarkGetFlagSet(b *testing.B) {
	cfg := &smallConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if fs := cfg.GetFlagSet(); fs == nil {
			b.Fatal("nil flag set")
		}
	}
}

// BenchmarkConvertStringEmptyMap measures ConvertString with an empty string
// and a map target (the Wave 2 empty-map path).
func BenchmarkConvertStringEmptyMap(b *testing.B) {
	mapType := reflect.TypeOf(map[string]string{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// NOTE: no error assertion — the pre-F7 baseline returns an error
		// here while the fixed tree returns an empty map; either way the
		// parse path cost is what is measured.
		_, _ = convert.ConvertString("", mapType, nil)
	}
}

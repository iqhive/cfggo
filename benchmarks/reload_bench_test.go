package benchmarks

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/iqhive/cfggo"
)

// ReloadConfig re-reads every configured source, re-applies precedence, diffs
// the result against the previous snapshot, and notifies listeners. These
// benchmarks measure that whole cycle for the different source backends.

func BenchmarkReloadMemHandler(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg,
		cfggo.WithoutEnv(),
		cfggo.WithConfigHandler(newMemHandler(mediumJSON)),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.ReloadConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReloadFile(b *testing.B) {
	dir := b.TempDir()
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte(mediumJSON), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}

	cfg := &mediumConfig{}
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

// BenchmarkReloadLargeMemHandler measures reload scaling against the 40-field
// struct (more keys to decode, diff, and re-apply).
func BenchmarkReloadLargeMemHandler(b *testing.B) {
	const largeJSON = `{
      "int0":100,"int1":101,"int2":102,"int3":103,"int4":104,
      "str0":"a","str1":"b","str2":"c","str3":"d","str4":"e",
      "bool0":false,"bool1":true,"flt0":9.5,"flt1":8.5,
      "dur0":"10s","dur1":"20s","slice0":["q","w"],"map0":{"z":9}
    }`
	cfg := &largeConfig{}
	mustInit(b, cfg,
		cfggo.WithoutEnv(),
		cfggo.WithConfigHandler(newMemHandler(largeJSON)),
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.ReloadConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReloadWithListeners measures reload cost with several OnChange
// listeners registered, since a reload diffs values and dispatches the
// aggregate change set to every listener.
func BenchmarkReloadWithListeners(b *testing.B) {
	for _, n := range []int{1, 8, 64} {
		b.Run(fmt.Sprintf("listeners=%d", n), func(b *testing.B) {
			cfg := &mediumConfig{}
			mustInit(b, cfg,
				cfggo.WithoutEnv(),
				cfggo.WithConfigHandler(newMemHandler(mediumJSON)),
			)
			var fired int
			for j := 0; j < n; j++ {
				cancel := cfg.OnChange(func(changes []cfggo.Change) { fired += len(changes) })
				defer cancel()
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := cfg.ReloadConfig(); err != nil {
					b.Fatal(err)
				}
			}
			_ = fired
		})
	}
}

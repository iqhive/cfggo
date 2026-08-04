package benchmarks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iqhive/cfggo"
)

// groupTestArgs isolates os.Args for Group benchmarks so the shared flag set
// never sees go test / benchmark flags.
func groupTestArgs(args []string) func() {
	old := os.Args
	os.Args = append([]string{"group-bench"}, args...)
	return func() { os.Args = old }
}

func BenchmarkStandaloneRead(b *testing.B) {
	cfg := &smallConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Port()
		_ = cfg.Host()
	}
}

func BenchmarkGroupMemberRead(b *testing.B) {
	restore := groupTestArgs(nil)
	defer restore()

	a, bb := &smallConfig{}, &smallConfig{}
	group := cfggo.NewGroup()
	if err := group.Register("a", a); err != nil {
		b.Fatalf("Register: %v", err)
	}
	if err := group.Register("b", bb); err != nil {
		b.Fatalf("Register: %v", err)
	}
	if err := group.Init(); err != nil {
		b.Fatalf("Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Port()
		_ = a.Host()
	}
}

func BenchmarkGroupInit2Members(b *testing.B) {
	restore := groupTestArgs(nil)
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a, bb := &smallConfig{}, &smallConfig{}
		group := cfggo.NewGroup()
		if err := group.Register("a", a); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := group.Register("b", bb); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := group.Init(); err != nil {
			b.Fatalf("Init: %v", err)
		}
	}
}

func BenchmarkGroupInit4Members(b *testing.B) {
	restore := groupTestArgs(nil)
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := cfggo.NewGroup()
		for _, ns := range []string{"a", "b", "c", "d"} {
			cfg := &smallConfig{}
			if err := group.Register(ns, cfg); err != nil {
				b.Fatalf("Register: %v", err)
			}
		}
		if err := group.Init(); err != nil {
			b.Fatalf("Init: %v", err)
		}
	}
}

func BenchmarkGroupInit2MembersWithFile(b *testing.B) {
	restore := groupTestArgs(nil)
	defer restore()

	dir := b.TempDir()
	file := filepath.Join(dir, "combined.json")
	combined := `{"a":{"port":9090,"host":"a.example"},"b":{"port":9091,"host":"b.example"}}`
	if err := os.WriteFile(file, []byte(combined), 0o644); err != nil {
		b.Fatalf("write config: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a, bb := &smallConfig{}, &smallConfig{}
		group := cfggo.NewGroup(cfggo.GroupWithFileConfig(file))
		if err := group.Register("a", a); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := group.Register("b", bb); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := group.Init(); err != nil {
			b.Fatalf("Init: %v", err)
		}
	}
}

package benchmarks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iqhive/cfggo"
)

// A Group is an Init-time orchestrator only: it initialises N Structures with
// coordinated options and never sits between an accessor and configData. These
// benchmarks back that claim up — the accessor benchmarks below must match their
// standalone counterparts, and Group.Init must cost about N standalone Inits
// plus one JSON split of the combined document.

// groupArgs replaces os.Args for the duration of a benchmark, since cfggo reads
// the process arguments when parsing flags.
func groupArgs(b *testing.B, args ...string) {
	b.Helper()
	original := os.Args
	os.Args = append([]string{"groupbench"}, args...)
	b.Cleanup(func() { os.Args = original })
}

func newGroup(b *testing.B, members int, options ...cfggo.GroupOption) *cfggo.Group {
	b.Helper()
	group := cfggo.NewGroup(options...)
	for i := 0; i < members; i++ {
		group.Register(namespaces[i], &mediumConfig{})
	}
	return group
}

var namespaces = []string{"svc0", "svc1", "svc2", "svc3", "svc4", "svc5", "svc6", "svc7"}

// --- Accessor reads: standalone vs. group member ---------------------------

func BenchmarkGroupMemberAccessorInt(b *testing.B) {
	groupArgs(b)
	cfg := &mediumConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	group.Register("svc0", cfg)
	if err := group.Init(); err != nil {
		b.Fatalf("Group.Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = cfg.Port()
	}
	_ = sink
}

func BenchmarkGroupMemberAccessorString(b *testing.B) {
	groupArgs(b)
	cfg := &mediumConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	group.Register("svc0", cfg)
	if err := group.Init(); err != nil {
		b.Fatalf("Group.Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.AppName()
	}
	_ = sink
}

// BenchmarkGroupMemberAccessorParallel shows that members keep independent
// mutexes: concurrent readers of different members do not contend.
func BenchmarkGroupMemberAccessorParallel(b *testing.B) {
	groupArgs(b)
	first := &mediumConfig{}
	second := &mediumConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	group.Register("svc0", first)
	group.Register("svc1", second)
	if err := group.Init(); err != nil {
		b.Fatalf("Group.Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		cfg := first
		if pb.Next() {
			cfg = second
		}
		var sink int
		for pb.Next() {
			sink = cfg.Port()
		}
		_ = sink
	})
}

// --- Group.Init scaling ----------------------------------------------------

func benchmarkGroupInit(b *testing.B, members int) {
	groupArgs(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := newGroup(b, members, cfggo.GroupWithoutFlags())
		if err := group.Init(); err != nil {
			b.Fatalf("Group.Init: %v", err)
		}
	}
}

func BenchmarkGroupInit1Member(b *testing.B)  { benchmarkGroupInit(b, 1) }
func BenchmarkGroupInit2Members(b *testing.B) { benchmarkGroupInit(b, 2) }
func BenchmarkGroupInit4Members(b *testing.B) { benchmarkGroupInit(b, 4) }
func BenchmarkGroupInit8Members(b *testing.B) { benchmarkGroupInit(b, 8) }

// BenchmarkGroupInitWithFlags includes registering every member's flags on the
// shared set and parsing it once.
func BenchmarkGroupInitWithFlags(b *testing.B) {
	groupArgs(b, "--svc0.port=7000", "--svc3.log_level=warn")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := newGroup(b, 4)
		if err := group.Init(); err != nil {
			b.Fatalf("Group.Init: %v", err)
		}
	}
}

// BenchmarkGroupInitWithFile includes reading the combined file, splitting it
// into per-namespace sections, and loading each section through the member's
// normal file path.
func BenchmarkGroupInitWithFile(b *testing.B) {
	groupArgs(b)
	document := `{"svc0":` + mediumJSON + `,"svc1":` + mediumJSON + `,"svc2":` + mediumJSON + `,"svc3":` + mediumJSON + `}`
	path := filepath.Join(b.TempDir(), "combined.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		b.Fatalf("write config: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		group := newGroup(b, 4, cfggo.GroupWithoutFlags(), cfggo.GroupWithFileConfig(path))
		if err := group.Init(); err != nil {
			b.Fatalf("Group.Init: %v", err)
		}
	}
}

// --- Group lifecycle operations -------------------------------------------

func BenchmarkGroupReload(b *testing.B) {
	groupArgs(b)
	document := `{"svc0":` + mediumJSON + `,"svc1":` + mediumJSON + `}`
	path := filepath.Join(b.TempDir(), "combined.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		b.Fatalf("write config: %v", err)
	}
	group := newGroup(b, 2, cfggo.GroupWithoutFlags(), cfggo.GroupWithFileConfig(path))
	if err := group.Init(); err != nil {
		b.Fatalf("Group.Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.Reload(); err != nil {
			b.Fatalf("Group.Reload: %v", err)
		}
	}
}

func BenchmarkGroupSave(b *testing.B) {
	groupArgs(b)
	path := filepath.Join(b.TempDir(), "combined.json")
	if err := os.WriteFile(path, []byte(`{"svc0":{},"svc1":{}}`), 0o600); err != nil {
		b.Fatalf("write config: %v", err)
	}
	group := newGroup(b, 2, cfggo.GroupWithoutFlags(), cfggo.GroupWithFileConfig(path))
	if err := group.Init(); err != nil {
		b.Fatalf("Group.Init: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := group.Save(); err != nil {
			b.Fatalf("Group.Save: %v", err)
		}
	}
}

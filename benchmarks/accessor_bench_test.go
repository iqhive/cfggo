package benchmarks

import (
	"testing"
	"time"

	"github.com/iqhive/cfggo"
)

// The typed accessor (cfg.Field()) is the hot path documented in the README:
// each call takes a read lock and returns a cached, already-converted value.
// These benchmarks measure that per-call cost across value types.

func BenchmarkAccessorInt(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = cfg.Port()
	}
	_ = sink
}

func BenchmarkAccessorGroupMemberInt(b *testing.B) {
	cfg := &mediumConfig{}
	group := cfggo.NewGroup()
	group.Register("service", cfg)
	if err := group.Init(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = cfg.Port()
	}
	_ = sink
}

func BenchmarkAccessorString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.AppName()
	}
	_ = sink
}

func BenchmarkAccessorBool(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink bool
	for i := 0; i < b.N; i++ {
		sink = cfg.Debug()
	}
	_ = sink
}

func BenchmarkAccessorFloat(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink float64
	for i := 0; i < b.N; i++ {
		sink = cfg.Rate()
	}
	_ = sink
}

func BenchmarkAccessorDuration(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink time.Duration
	for i := 0; i < b.N; i++ {
		sink = cfg.Timeout()
	}
	_ = sink
}

func BenchmarkAccessorSlice(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink []string
	for i := 0; i < b.N; i++ {
		sink = cfg.Tags()
	}
	_ = sink
}

func BenchmarkAccessorMap(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink map[string]string
	for i := 0; i < b.N; i++ {
		sink = cfg.Labels()
	}
	_ = sink
}

// BenchmarkAccessorNested reads a value through a two-level nested sub-struct
// accessor (server.db.host).
func BenchmarkAccessorNested(b *testing.B) {
	cfg := &nestedConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.Server.DB.Host()
	}
	_ = sink
}

// BenchmarkAccessorParallel measures read throughput under contention: many
// goroutines hammering the same accessor (and therefore the same RWMutex).
func BenchmarkAccessorParallel(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var sink int
		for pb.Next() {
			sink = cfg.Port()
		}
		_ = sink
	})
}

// BenchmarkGetByKey measures the dynamic, string-keyed read API used by admin
// endpoints and tooling, for comparison with the typed accessor above.
func BenchmarkGetByKey(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cfg.Get("port")
	}
}

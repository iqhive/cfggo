package benchmarks

import (
	"sync/atomic"
	"testing"

	"github.com/iqhive/cfggo"
)

// cfggo guards each instance with a per-instance RWMutex. These benchmarks
// characterise behaviour under concurrency: pure reads (RLock throughput),
// pure writes (Lock contention), and a realistic read-heavy mix.
//
// Note on error handling: testing.B.Fatal must only be called from the
// goroutine running the benchmark, never from RunParallel worker goroutines.
// Workers therefore record failures in an atomic flag that the main goroutine
// inspects after RunParallel returns.

// BenchmarkConcurrentReads is the best case for an RWMutex: many readers, no
// writers. Throughput should scale with GOMAXPROCS.
func BenchmarkConcurrentReads(b *testing.B) {
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

// BenchmarkConcurrentWrites is the worst case: every goroutine takes the write
// lock via Set.
func BenchmarkConcurrentWrites(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	var failed atomic.Bool
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			if err := cfg.Set("port", i); err != nil {
				failed.Store(true)
				return
			}
			i++
		}
	})
	if failed.Load() {
		b.Fatal("Set failed during concurrent writes")
	}
}

// BenchmarkConcurrentReadWrite models a read-heavy workload (roughly 1 writer
// for every 16 readers) sharing one config instance, which is the typical
// hot-reload scenario: occasional writes, constant reads.
func BenchmarkConcurrentReadWrite(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	var counter int64
	var failed atomic.Bool
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var sink int
		for pb.Next() {
			if atomic.AddInt64(&counter, 1)%16 == 0 {
				if err := cfg.Set("port", sink); err != nil {
					failed.Store(true)
					return
				}
			} else {
				sink = cfg.Port()
			}
		}
		_ = sink
	})
	if failed.Load() {
		b.Fatal("Set failed during concurrent read/write")
	}
}

// BenchmarkConcurrentIndependentInstances confirms that separate Structure
// instances do not contend on a shared lock: each goroutine reads its own
// config, so throughput should not degrade from locking.
func BenchmarkConcurrentIndependentInstances(b *testing.B) {
	var failed atomic.Bool
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		cfg := &mediumConfig{}
		if err := cfggo.Init(cfg, cfggo.WithSkipEnvironment()); err != nil {
			failed.Store(true)
			return
		}
		var sink int
		for pb.Next() {
			sink = cfg.Port()
		}
		_ = sink
	})
	if failed.Load() {
		b.Fatal("Init failed for an independent instance")
	}
}

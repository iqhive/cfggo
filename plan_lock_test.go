package cfggo

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// planSeedRaceCfg exposes two accessor keys. Alpha's caller-supplied default
// func blocks until the concurrent reader is looping, so the seed writes are
// guaranteed to overlap the reads (the window -race must instrument).
type planSeedRaceCfg struct {
	Structure
	Alpha func() string
	Beta  func() string
}

// TestPlanSeedConcurrentRead hammers Get/Sources from another goroutine while
// Init seeds defaults. The seed writes must hold configMutex: otherwise -race
// reports a data race on configData/provenance, and a reader can observe a
// half-applied state (exactly one of the two keys present).
func TestPlanSeedConcurrentRead(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var enterOnce sync.Once

	cfg := &planSeedRaceCfg{}
	cfg.Alpha = func() string {
		enterOnce.Do(func() { close(entered) })
		<-release
		return "a"
	}
	cfg.Beta = DefaultValue("b")

	initDone := make(chan error, 1)
	go func() { initDone <- cfg.Init(cfg, WithoutFlags()) }()

	// Wait until the seed loop is inside Alpha's default func, then start a
	// tight reader loop before releasing it.
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("seed loop never reached the blocking default func")
	}

	var stop atomic.Bool
	var halfSeen atomic.Bool
	var loopCount atomic.Int64
	readerReady := make(chan struct{})
	var startOnce sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			loopCount.Add(1)
			// Snapshot check under read lock:
			// No reader holding the config lock may ever observe a half-applied
			// seed (Alpha present in configData without Beta, or configData populated
			// while defaultData or provenance is absent/stale).
			cfg.configMutex.RLock()
			_, hasCA := cfg.configData["Alpha"]
			_, hasCB := cfg.configData["Beta"]
			_, hasDA := cfg.defaultData["Alpha"]
			_, hasDB := cfg.defaultData["Beta"]
			if hasCA != hasCB || hasDA != hasDB {
				halfSeen.Store(true)
			}
			if hasCA && (!hasDA || !hasDB || len(cfg.configData) != len(cfg.defaultData) || len(cfg.configData) != len(cfg.provenance)) {
				halfSeen.Store(true)
			}
			cfg.configMutex.RUnlock()

			// Sources snapshot check:
			// Sources() takes configMutex.RLock internally. Both keys must appear
			// together atomically in the provenance map.
			srcs := cfg.Sources()
			_, srcA := srcs["Alpha"]
			_, srcB := srcs["Beta"]
			if srcA != srcB {
				halfSeen.Store(true)
			}

			// Sequential Get check:
			// Alpha is ordered before Beta in the seed sequence. If a reader
			// observes Alpha present, the atomic seed has already committed, so
			// Beta must also be present. Observing Alpha present but Beta absent
			// proves a half-applied write window.
			_, aOK := cfg.Get("Alpha")
			_, bOK := cfg.Get("Beta")
			if aOK && !bOK {
				halfSeen.Store(true)
			}

			startOnce.Do(func() { close(readerReady) })
		}
	}()
	<-readerReady
	close(release)

	select {
	case err := <-initDone:
		if err != nil {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("Init: %v", err)
		}
	case <-time.After(30 * time.Second):
		stop.Store(true)
		wg.Wait()
		t.Fatal("Init did not complete (possible deadlock in seed loop)")
	}
	stop.Store(true)
	wg.Wait()

	if halfSeen.Load() {
		t.Fatal("reader observed a half-applied seed: exactly one of Alpha/Beta was present")
	}
	if loopCount.Load() == 0 {
		t.Fatal("reader loop never executed")
	}
	if got := cfg.Alpha(); got != "a" {
		t.Fatalf("Alpha() = %q, want %q", got, "a")
	}
	if got := cfg.Beta(); got != "b" {
		t.Fatalf("Beta() = %q, want %q", got, "b")
	}
}

// planSeedSelfReadCfg's default func for Beta reads config itself. Default
// funcs are arbitrary caller code, so they must execute WITHOUT the config
// lock held: this Init must complete (not deadlock) under a timeout.
type planSeedSelfReadCfg struct {
	Structure
	A func() string
	B func() string
}

func TestPlanSeedDefaultFuncMayReadConfig(t *testing.T) {
	cfg := &planSeedSelfReadCfg{}
	cfg.A = DefaultValue("a")
	cfg.B = func() string {
		// Must not deadlock: proves no default func runs under configMutex.
		// Note: seeds are staged, so keys seeded by other defaults are NOT
		// yet visible here (Get("A") misses); the func just must complete.
		if v, ok := cfg.Get("A"); ok {
			if s, ok := v.(string); ok && s != "" {
				return s + "+b"
			}
		}
		// A is not seeded yet while defaults are being computed; that is
		// fine, the func just must complete.
		if err := cfg.Set("B", "b"); err == nil {
			return "b"
		}
		return "b"
	}

	done := make(chan error, 1)
	go func() { done <- cfg.Init(cfg, WithoutFlags()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Init deadlocked: a default func that reads config did not complete")
	}
	if got := cfg.B(); got != "b" {
		t.Fatalf("B() = %q, want %q", got, "b")
	}
}

package cfggo

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// RED: deterministic lost-update proof. Reload A stages v1 and blocks in its
// validator; reload B then runs to completion with v2; releasing A must NOT let
// A's stale candidate clobber B's fresher commit.
func TestReloadConcurrentReloadDoesNotClobberNewerCommit(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090,"host":"old"}`)}
	cfg := initReloadValidatorConfig(t, handler)

	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	cfg.RegisterValidator("port", func(interface{}) error {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-time.After(5 * time.Second):
			}
		}
		return nil
	})

	doneA := make(chan error, 1)
	go func() { doneA <- cfg.Reload() }()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reload A never reached validation")
	}

	// B reloads a newer source value while A is parked in validation.
	handler.data = json.RawMessage(`{"port":9191,"host":"old"}`)
	doneB := make(chan error, 1)
	go func() { doneB <- cfg.Reload() }()
	select {
	case err := <-doneB:
		if err != nil {
			t.Fatalf("reload B: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload B deadlocked")
	}
	if got := cfg.Port(); got != 9191 {
		t.Fatalf("Port() after B = %d, want 9191", got)
	}

	close(release)
	select {
	case err := <-doneA:
		if err != nil {
			t.Fatalf("reload A: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload A deadlocked")
	}

	// A's stale candidate (9090) must not clobber B's fresher commit (9191).
	if got := cfg.Port(); got != 9191 {
		t.Fatalf("LOST UPDATE: Port() = %d, want 9191 (reload A clobbered reload B)", got)
	}
}

// Stress: N concurrent reloaders x M setters — no deadlock, no DATA RACE
// (run under -race), and no lost writes: a Set pinned after the storm must
// survive a final Reload via the validation-window delta re-assert.
func TestReloadConcurrentStormNoDeadlockNoLostWrites(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090,"host":"old"}`)}
	cfg := initReloadValidatorConfig(t, handler)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_ = cfg.Reload()
				}
			}()
		}
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_ = cfg.Set("host", "setter")
				}
			}()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("stress: deadlock under concurrent reload/Set")
	}
	// No lost writes: a Set pinned after the storm must still be live after
	// a final Reload (the validation-window delta re-assert carries it over
	// the commit). The storm validator writes host too, but every storm
	// goroutine has joined by now, so no validator is still in flight.
	if err := cfg.Set("host", "pinned-final"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := cfg.Reload(); err != nil {
		t.Fatalf("final Reload: %v", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want 9090", got)
	}
	if got := cfg.Host(); got != "pinned-final" {
		t.Fatalf("Host() = %q, want pinned-final (no lost write)", got)
	}
}

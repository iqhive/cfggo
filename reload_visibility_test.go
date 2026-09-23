package cfggo

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type reloadVisibilityConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"1"`
	Host func() string `cfggo:"host" default:"default-host"`
}

// slowHandler is a ConfigHandler whose LoadConfig takes a while, so a reload
// spends measurable time between its first and last state mutation.
type slowHandler struct {
	data  json.RawMessage
	delay time.Duration
}

func (h *slowHandler) IsDefault() bool { return false }
func (h *slowHandler) LoadConfig() (json.RawMessage, error) {
	time.Sleep(h.delay)
	return h.data, nil
}
func (h *slowHandler) SaveConfig(json.RawMessage) error { return nil }

// failingHandler returns err from LoadConfig once it is set.
type failingHandler struct {
	data json.RawMessage
	err  error
}

func (h *failingHandler) IsDefault() bool { return false }
func (h *failingHandler) LoadConfig() (json.RawMessage, error) {
	if h.err != nil {
		return nil, h.err
	}
	return h.data, nil
}
func (h *failingHandler) SaveConfig(json.RawMessage) error { return nil }

// Accessors must never observe an intermediate reload state: before the fix,
// Reload reset the live map to the defaults and rebuilt it in place across
// several lock acquisitions, so a concurrent cfg.Port() could briefly return
// the default (1) instead of the current or new value.
func TestReloadNeverExposesIntermediateStateToReaders(t *testing.T) {
	handler := &slowHandler{data: json.RawMessage(`{"port": 9090, "host": "file-host"}`), delay: 20 * time.Millisecond}
	cfg := &reloadVisibilityConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want 9090", got)
	}

	var bad atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if p := cfg.Port(); p != 9090 {
					bad.Add(1)
				}
				if h := cfg.Host(); h != "file-host" {
					bad.Add(1)
				}
				if v, ok := cfg.Get("port"); !ok || v != 9090 {
					bad.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		if err := cfg.Reload(); err != nil {
			t.Fatalf("Reload: %v", err)
		}
	}
	close(stop)
	wg.Wait()
	if n := bad.Load(); n != 0 {
		t.Fatalf("readers observed an intermediate reload state %d times", n)
	}
}

// A reload whose source fails must leave live state, provenance and the dirty
// flag exactly as they were, including a pending Set.
func TestReloadFailureLeavesLiveStateUntouched(t *testing.T) {
	handler := &failingHandler{data: json.RawMessage(`{"port": 9090}`)}
	cfg := &reloadVisibilityConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("host", "set-host"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	handler.err = os.ErrNotExist
	if err := cfg.Reload(); err == nil {
		t.Fatal("Reload succeeded with a failing source")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() after failed reload = %d, want 9090", got)
	}
	if got := cfg.Host(); got != "set-host" {
		t.Fatalf("Host() after failed reload = %q, want set-host", got)
	}
	if src, _ := cfg.Source("host"); src != SourceSet {
		t.Fatalf("Source(host) = %v, want set", src)
	}
	if changed, _ := cfg.changedState(); !changed {
		t.Fatal("pending Set was no longer marked as unsaved after a failed reload")
	}
}

// The environment layer applied during Reload must still win over the file
// layer and be re-read on every reload.
func TestReloadRereadsEnvironmentUnderLock(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port": 9090, "host": "file-host"}`)}
	cfg := &reloadVisibilityConfig{}
	t.Setenv("RVT_HOST", "env-host")
	if err := cfg.Init(cfg, WithoutFlags(), WithEnvPrefix("RVT_"), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "env-host" {
		t.Fatalf("Host() = %q, want env-host", got)
	}
	t.Setenv("RVT_HOST", "env-host-2")
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "env-host-2" {
		t.Fatalf("Host() after reload = %q, want env-host-2", got)
	}
	if src, _ := cfg.Source("host"); src != SourceEnv {
		t.Fatalf("Source(host) = %v, want env", src)
	}
	os.Unsetenv("RVT_HOST")
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "file-host" {
		t.Fatalf("Host() after env removed = %q, want file-host", got)
	}
}

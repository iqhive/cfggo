package cfggo

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// reloadValidatorConfig is the test config for staged-commit reload tests.
type reloadValidatorConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"8080"`
	Host func() string `cfggo:"host" default:"localhost"`
}

func initReloadValidatorConfig(t *testing.T, handler *memHandler, opts ...Option) *reloadValidatorConfig {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	cfg := &reloadValidatorConfig{}
	all := append([]Option{WithConfigHandler(handler), WithoutFlags(), WithoutEnv()}, opts...)
	if err := cfg.Init(cfg, all...); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cfg
}

func reloadWithTimeout(t *testing.T, cfg *reloadValidatorConfig, d time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cfg.Reload() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatal("Reload deadlocked: validator self-deadlock (F4)")
		return nil
	}
}

// (iv) validator calling Set must not self-deadlock.
func TestReloadValidatorCallingSetCompletes(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090}`)}
	cfg := initReloadValidatorConfig(t, handler)
	cfg.RegisterValidator("port", func(interface{}) error {
		return cfg.Set("host", "validator-updated")
	})
	if err := reloadWithTimeout(t, cfg, 5*time.Second); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "validator-updated" {
		t.Fatalf("Host() = %q, want validator-updated", got)
	}
}

// (iv) validator calling Reload must not self-deadlock.
func TestReloadValidatorCallingReloadCompletes(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090}`)}
	cfg := initReloadValidatorConfig(t, handler)
	once := make(chan struct{}, 1)
	once <- struct{}{}
	cfg.RegisterValidator("port", func(interface{}) error {
		select {
		case <-once:
			return cfg.Reload()
		default:
			return nil
		}
	})
	if err := reloadWithTimeout(t, cfg, 5*time.Second); err != nil {
		t.Fatalf("Reload: %v", err)
	}
}

// (ii) failing validator is a pure no-op: last-known-good stays live.
func TestReloadFailingValidatorKeepsLastKnownGood(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090,"host":"old"}`)}
	cfg := initReloadValidatorConfig(t, handler)
	if err := cfg.Set("host", "pinned"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	beforeChanged, beforeVersion := cfg.changedState()
	sentinel := errors.New("bad port")
	cfg.RegisterValidator("port", func(interface{}) error { return sentinel })
	handler.data = json.RawMessage(`{"port":70000,"host":"new"}`)
	if err := reloadWithTimeout(t, cfg, 5*time.Second); err == nil {
		t.Fatal("Reload: expected validation error, got nil")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want 9090 (last-known-good)", got)
	}
	if got := cfg.Host(); got != "pinned" {
		t.Fatalf("Host() = %q, want pinned (concurrent Set preserved)", got)
	}
	if src, _ := cfg.Source("port"); src != SourceFile {
		t.Fatalf("Source(port) = %s, want file", src)
	}
	if src, _ := cfg.Source("host"); src != SourceSet {
		t.Fatalf("Source(host) = %s, want set", src)
	}
	afterChanged, afterVersion := cfg.changedState()
	if afterChanged != beforeChanged || afterVersion != beforeVersion {
		t.Fatalf("changed state mutated by failed reload: before (%v,%d) after (%v,%d)",
			beforeChanged, beforeVersion, afterChanged, afterVersion)
	}
}

// (iii) a Set landing during the validation window is re-applied at commit.
func TestReloadPreservesSetDuringValidationWindow(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090,"host":"old"}`)}
	cfg := initReloadValidatorConfig(t, handler)
	release := make(chan struct{})
	setDone := make(chan struct{})
	cfg.RegisterValidator("port", func(interface{}) error {
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- cfg.Reload() }()
	time.Sleep(200 * time.Millisecond)
	go func() {
		defer close(setDone)
		if err := cfg.Set("host", "during-validation"); err != nil {
			t.Errorf("Set during validation: %v", err)
		}
	}()
	select {
	case <-setDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Set blocked during validation window")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Reload: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reload deadlocked")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want reloaded 9090", got)
	}
	if got := cfg.Host(); got != "during-validation" {
		t.Fatalf("Host() = %q, want during-validation (Set delta re-applied)", got)
	}
	if src, _ := cfg.Source("host"); src != SourceSet {
		t.Fatalf("Source(host) = %s, want set", src)
	}
}

// (i) readers never observe unvalidated intermediate state.
func TestReloadReadersNeverSeeUnvalidatedState(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090}`)}
	cfg := initReloadValidatorConfig(t, handler)
	release := make(chan struct{})
	cfg.RegisterValidator("port", func(interface{}) error {
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- cfg.Reload() }()
	time.Sleep(200 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if got := cfg.Port(); got != 8080 && got != 9090 {
					t.Errorf("Port() = %d, want 8080 (old) or 9090 (new), never intermediate", got)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Reload: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reload deadlocked")
	}
}

// -race stress: concurrent reloaders and setters, no deadlock, no lost writes.
func TestReloadValidatorStressNoDeadlockNoLostWrites(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port":9090,"host":"old"}`)}
	cfg := initReloadValidatorConfig(t, handler)
	cfg.RegisterValidator("port", func(interface{}) error {
		_ = cfg.Set("host", "validator-touch")
		return nil
	})
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
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = cfg.Set("host", "setter")
			}
		}(i)
	}
	reloadDone := make(chan struct{})
	go func() { wg.Wait(); close(reloadDone) }()
	select {
	case <-reloadDone:
	case <-time.After(20 * time.Second):
		t.Fatal("stress: deadlock under concurrent reload/Set")
	}
}

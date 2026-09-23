package cfggo

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type versionConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"1"`
	Host func() string `cfggo:"host" default:"h"`
}

// parkedSaveHandler parks SaveConfig until released so a Save can be held
// open while other operations run.
type parkedSaveHandler struct {
	data    json.RawMessage
	entered chan struct{}
	release chan struct{}
	saved   json.RawMessage
}

func (h *parkedSaveHandler) IsDefault() bool                      { return false }
func (h *parkedSaveHandler) LoadConfig() (json.RawMessage, error) { return h.data, nil }
func (h *parkedSaveHandler) SaveConfig(b json.RawMessage) error {
	h.saved = b
	if h.entered != nil {
		close(h.entered)
		h.entered = nil
		select {
		case <-h.release:
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

// The change version must never move backwards across a reload commit. A
// Save that captured the live version during a reload's validation window
// and is still writing when a later Set lands and the reload commits must
// not be able to clear the dirty flag over that later, unsaved Set.
func TestReloadCommitKeepsChangeVersionMonotonic(t *testing.T) {
	handler := &parkedSaveHandler{
		data:    json.RawMessage(`{"port": 9090}`),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	saveEntered := handler.entered
	cfg := &versionConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}

	validatorEntered := make(chan struct{})
	validatorRelease := make(chan struct{})
	var calls atomic.Int32
	cfg.RegisterValidator("port", func(interface{}) error {
		if calls.Add(1) == 1 {
			close(validatorEntered)
			select {
			case <-validatorRelease:
			case <-time.After(5 * time.Second):
			}
		}
		return nil
	})

	reloadDone := make(chan error, 1)
	go func() { reloadDone <- cfg.Reload() }()
	select {
	case <-validatorEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("reload never reached validation")
	}

	// Validation window: Set x, start a Save that captures the version and
	// blocks in the handler, Set y (unsaved), then let the reload commit
	if err := cfg.Set("host", "x"); err != nil {
		t.Fatal(err)
	}
	saveDone := make(chan error, 1)
	go func() { saveDone <- cfg.Save() }()
	select {
	case <-saveEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("Save never reached the handler")
	}
	if err := cfg.Set("host", "y"); err != nil {
		t.Fatal(err)
	}
	close(validatorRelease)
	select {
	case err := <-reloadDone:
		if err != nil {
			t.Fatalf("Reload: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload deadlocked")
	}

	// The commit has happened; now the Save finishes and tries to clear the
	// dirty flag with the version it captured before Set(host, y)
	close(handler.release)
	select {
	case err := <-saveDone:
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Save deadlocked")
	}

	if got := cfg.Host(); got != "y" {
		t.Fatalf("Host() = %q, want y", got)
	}
	if !strings.Contains(string(handler.saved), `"host":"x"`) {
		t.Fatalf("the blocked Save should have written x: %s", handler.saved)
	}
	if changed, _ := cfg.changedState(); !changed {
		t.Fatal("the unsaved Set(host, y) is no longer marked as unsaved: the reload commit rewound the change version")
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if !strings.Contains(string(handler.saved), `"host":"y"`) {
		t.Fatalf("SaveIfChanged did not persist the unsaved Set: %s", handler.saved)
	}
}

// A key registered with NewFlag while a reload is in its validation window
// must survive the reload's commit.
func TestNewFlagDuringReloadWindowSurvivesCommit(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"port": 9090}`)}
	cfg := &versionConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(), WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}

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

	done := make(chan error, 1)
	go func() { done <- cfg.Reload() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reload never reached validation")
	}
	cfg.NewFlag("late", "v", "registered mid-reload")
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Reload: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload deadlocked")
	}
	if v, ok := cfg.Get("late"); !ok || v != "v" {
		t.Fatalf("Get(late) = %v, %v; want v, true (key dropped by the reload commit)", v, ok)
	}
	if src, _ := cfg.Source("late"); src != SourceDefault {
		t.Fatalf("Source(late) = %v, want default", src)
	}
}

type secretDefaultConfig struct {
	Structure
	Token func() string `cfggo:"token" default:"hunter2" secret:"true"`
	Port  func() int    `cfggo:"port" default:"1"`
}

// After an Init that fails once the accessors are wired (here: a bad
// environment value), the default values stay live. They must still be
// recognised and secret ones must still be masked in the dumps.
func TestFailedInitKeepsSecretDefaultsMaskedAndRecognized(t *testing.T) {
	t.Setenv("SDC_PORT", "not-a-number")
	cfg := &secretDefaultConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithEnvPrefix("SDC_")); err == nil {
		t.Fatal("Init should have failed on the bad environment value")
	}
	if got := cfg.Token(); got != "hunter2" {
		t.Fatalf("Token() = %q, want the default", got)
	}
	for name, dump := range map[string]string{"String": cfg.String(), "Explain": cfg.Explain(), "Report": cfg.Report()} {
		if strings.Contains(dump, "hunter2") {
			t.Fatalf("%s() printed the secret default after a failed Init:\n%s", name, dump)
		}
	}
	if d := cfg.DiagnoseData(); len(d.Unrecognized) != 0 {
		t.Fatalf("Diagnose reported the default keys as unrecognized after a failed Init: %v", d.Unrecognized)
	}
}

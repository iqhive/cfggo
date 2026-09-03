package cfggo

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

type saveSemanticsConfig struct {
	Structure
	Port     func() int    `cfggo:"port" default:"1"`
	Name     func() string `cfggo:"name" default:"svc"`
	Password func() string `cfggo:"db_password" secret:"true"`
	Level    func() string `cfggo:"level" default:"info"`
}

// countingHandler serves a fixed document and records every save
type countingHandler struct {
	mu    sync.Mutex
	data  json.RawMessage
	saved [][]byte
}

func (h *countingHandler) IsDefault() bool { return false }
func (h *countingHandler) LoadConfig() (json.RawMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.data, nil
}
func (h *countingHandler) SaveConfig(data json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.saved = append(h.saved, append([]byte(nil), data...))
	return nil
}
func (h *countingHandler) saves() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.saved)
}
func (h *countingHandler) lastSaved(t *testing.T) map[string]interface{} {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.saved) == 0 {
		t.Fatal("nothing was saved")
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(h.saved[len(h.saved)-1], &doc); err != nil {
		t.Fatalf("saved document is not JSON: %v (%s)", err, h.saved[len(h.saved)-1])
	}
	return doc
}
func (h *countingHandler) setData(data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = json.RawMessage(data)
}

func TestLoadingLayersDoNotMarkConfigDirty(t *testing.T) {
	setProcessArgs(t, "--port=9")
	t.Setenv("DB_PASSWORD", "env-secret")
	handler := &countingHandler{data: json.RawMessage(`{"name":"fromfile"}`)}
	cfg := &saveSemanticsConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if cfg.Port() != 9 || cfg.Password() != "env-secret" || cfg.Name() != "fromfile" {
		t.Fatalf("overrides not applied: port=%d password=%q name=%q", cfg.Port(), cfg.Password(), cfg.Name())
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if n := handler.saves(); n != 0 {
		t.Fatalf("SaveIfChanged after Init wrote %d time(s); file, env and flag loading must not make the config dirty", n)
	}
	if err := cfg.Set("level", "debug"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged after Set: %v", err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("second SaveIfChanged: %v", err)
	}
	if n := handler.saves(); n != 1 {
		t.Fatalf("saves after Set = %d, want exactly 1", n)
	}
}

func TestSaveOmitsRuntimeOverridesAndEnvSecrets(t *testing.T) {
	setProcessArgs(t, "--port=9")
	t.Setenv("DB_PASSWORD", "env-secret")
	t.Setenv("NAME", "env-name")
	handler := &countingHandler{data: json.RawMessage(`{"name":"fromfile","level":"warn"}`)}
	cfg := &saveSemanticsConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("level", "debug"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	doc := handler.lastSaved(t)
	if got := doc["port"]; got != float64(1) {
		t.Errorf("saved port = %v, want the default 1: the --port flag must not be persisted", got)
	}
	if got := doc["name"]; got != "fromfile" {
		t.Errorf("saved name = %v, want the file value: the NAME env override must not be persisted", got)
	}
	if got := doc["db_password"]; got != "" {
		t.Errorf("saved db_password = %v, want the empty default: the env secret must not be persisted", got)
	}
	if got := doc["level"]; got != "debug" {
		t.Errorf("saved level = %v, want the Set value", got)
	}
	if raw := string(handler.saved[0]); strings.Contains(raw, "env-secret") {
		t.Errorf("saved document contains the env secret: %s", raw)
	}

	// The live state and GetJSONBytes keep the overrides
	if cfg.Port() != 9 || cfg.Name() != "env-name" || cfg.Password() != "env-secret" {
		t.Errorf("live values changed by Save: port=%d name=%q password=%q", cfg.Port(), cfg.Name(), cfg.Password())
	}
	live, err := cfg.GetJSONBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(live), `"port":9`) || !strings.Contains(string(live), "env-secret") {
		t.Errorf("GetJSONBytes = %s, want the full live state including overrides", live)
	}
}

func TestSaveClearsDirtyFlag(t *testing.T) {
	setProcessArgs(t)
	handler := &countingHandler{data: json.RawMessage(`{}`)}
	cfg := &saveSemanticsConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("level", "debug"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if n := handler.saves(); n != 1 {
		t.Fatalf("saves = %d, want 1: Save must clear the dirty flag", n)
	}
}

func TestReloadKeepsUnsavedSetPending(t *testing.T) {
	setProcessArgs(t)
	t.Setenv("NAME", "env-name")
	handler := &countingHandler{data: json.RawMessage(`{"name":"fromfile","level":"warn"}`)}
	cfg := &saveSemanticsConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("level", "debug"); err != nil {
		t.Fatal(err)
	}
	handler.setData(`{"name":"newfile","level":"error"}`)
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Level(); got != "debug" {
		t.Fatalf("Level() after reload = %q, want the Set value re-asserted", got)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if n := handler.saves(); n != 1 {
		t.Fatalf("saves = %d, want 1: an unsaved Set stays pending across a reload", n)
	}
	doc := handler.lastSaved(t)
	if got := doc["name"]; got != "newfile" {
		t.Errorf("saved name = %v, want the value the reloaded file supplied (env override omitted)", got)
	}
	if got := doc["level"]; got != "debug" {
		t.Errorf("saved level = %v, want the Set value", got)
	}

	// A reload with nothing pending leaves the config clean
	if err := cfg.Reload(); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatal(err)
	}
	if n := handler.saves(); n != 1 {
		t.Fatalf("saves after a clean reload = %d, want still 1", n)
	}
}

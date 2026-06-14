package cfggo

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

type mapLeafRegressionConfig struct {
	Structure
	Labels   func() map[string]string `cfggo:"labels"`
	Metadata func() map[string]int    `cfggo:"metadata"`
}

func TestMapLeafLoadsFromJSONObject(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{
		"labels":{"env":"prod","team":"core"},
		"metadata":{"retries":3}
	}`)}
	cfg := &mapLeafRegressionConfig{
		Labels:   DefaultValue(map[string]string{}),
		Metadata: DefaultValue(map[string]int{}),
	}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	labels := cfg.Labels()
	if labels["env"] != "prod" || labels["team"] != "core" {
		t.Fatalf("Labels() = %#v, want env/team loaded from JSON object", labels)
	}
	metadata := cfg.Metadata()
	if metadata["retries"] != 3 {
		t.Fatalf("Metadata() = %#v, want retries=3", metadata)
	}
	if _, ok := cfg.Get("labels.env"); ok {
		t.Fatal("JSON object leaf was flattened into labels.env")
	}
}

type mutableAccessorRegressionConfig struct {
	Structure
	Labels func() map[string]string `cfggo:"labels"`
	Names  func() []string          `cfggo:"names"`
}

func TestMutableAccessorsReturnCopies(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &mutableAccessorRegressionConfig{
		Labels: DefaultValue(map[string]string{"env": "prod"}),
		Names:  DefaultValue([]string{"api"}),
	}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	labels := cfg.Labels()
	labels["env"] = "dev"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating accessor result, want prod", got)
	}

	names := cfg.Names()
	names[0] = "worker"
	if got := cfg.Names()[0]; got != "api" {
		t.Fatalf("Names()[0] = %q after mutating accessor result, want api", got)
	}

	labelsValue, ok := Value[map[string]string](&cfg.Structure, "labels")
	if !ok {
		t.Fatal("Value[map[string]string](labels) returned ok=false")
	}
	labelsValue["env"] = "qa"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating typed Value result, want prod", got)
	}
}

type reloadDefaultsRegressionConfig struct {
	Structure
	Host func() string `cfggo:"host" default:"localhost"`
}

func TestReloadRemovedFileKeyRevertsToDefault(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{"host":"fromfile"}`)}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "fromfile" {
		t.Fatalf("initial Host() = %q, want fromfile", got)
	}

	handler.data = json.RawMessage(`{}`)
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "localhost" {
		t.Fatalf("after reload Host() = %q, want default localhost", got)
	}
	if src, _ := cfg.Source("host"); src != SourceDefault {
		t.Fatalf("after reload source = %s, want default", src)
	}
}

type setOverrideRegressionConfig struct {
	Structure
	Host func() string `cfggo:"host" default:"localhost"`
}

func TestReloadPreservesProgrammaticSetOverride(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("HOST", "fromenv")

	cfg := &setOverrideRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "fromenv" {
		t.Fatalf("initial Host() = %q, want fromenv", got)
	}
	if err := cfg.Set("host", "runtime-override"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "runtime-override" {
		t.Fatalf("after reload Host() = %q, want runtime override", got)
	}
	if src, _ := cfg.Source("host"); src != SourceSet {
		t.Fatalf("after reload source = %s, want set", src)
	}
}

type lazyInitRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestConcurrentLazyInitIsSerialized(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &lazyInitRegressionConfig{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cfg.Get("port")
		}()
	}
	wg.Wait()
}

type strictNumericRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestStrictJSONLoadRejectsLossyNumericConversion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &strictNumericRegressionConfig{}
	err := cfg.Init(cfg, WithConfigHandler(&memHandler{data: json.RawMessage(`{"port": 8080.5}`)}), WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected lossy numeric conversion error, got nil")
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want default retained after failed load", got)
	}
}

func TestLoadConversionErrorIncludesKeyAndSource(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &strictNumericRegressionConfig{}
	err := cfg.Init(cfg, WithConfigHandler(&memHandler{data: json.RawMessage(`{"port": "not-a-number"}`)}), WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected conversion error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{`key "port"`, "from file"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

type blockingSaveHandler struct {
	data        json.RawMessage
	saveStarted chan struct{}
	releaseSave chan struct{}
	mu          sync.Mutex
	saves       int
}

func (h *blockingSaveHandler) IsDefault() bool { return false }

func (h *blockingSaveHandler) LoadConfig() (json.RawMessage, error) {
	return h.data, nil
}

func (h *blockingSaveHandler) SaveConfig(data json.RawMessage) error {
	select {
	case h.saveStarted <- struct{}{}:
	default:
	}
	<-h.releaseSave
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = data
	h.saves++
	return nil
}

func (h *blockingSaveHandler) saveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saves
}

func TestSaveIfChangedPreservesConcurrentSet(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &blockingSaveHandler{
		data:        json.RawMessage(`{}`),
		saveStarted: make(chan struct{}, 2),
		releaseSave: make(chan struct{}),
	}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("host", "first"); err != nil {
		t.Fatalf("Set first: %v", err)
	}

	saveDone := make(chan error, 1)
	go func() { saveDone <- cfg.SaveIfChanged() }()
	<-handler.saveStarted

	if err := cfg.Set("host", "second"); err != nil {
		t.Fatalf("Set second: %v", err)
	}
	close(handler.releaseSave)
	if err := <-saveDone; err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("second SaveIfChanged: %v", err)
	}
	if got := handler.saveCount(); got != 2 {
		t.Fatalf("SaveConfig calls = %d, want 2", got)
	}
}

type failingReloadHandler struct {
	data  json.RawMessage
	fail  bool
	mu    sync.Mutex
	saves int
}

func (h *failingReloadHandler) IsDefault() bool { return false }

func (h *failingReloadHandler) LoadConfig() (json.RawMessage, error) {
	if h.fail {
		return nil, errors.New("reload failed")
	}
	return h.data, nil
}

func (h *failingReloadHandler) SaveConfig(data json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = data
	h.saves++
	return nil
}

func (h *failingReloadHandler) saveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saves
}

func TestFailedReloadPreservesDirtyState(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &failingReloadHandler{data: json.RawMessage(`{"host":"fromfile"}`)}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("host", "runtime"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	handler.fail = true
	if err := cfg.Reload(); err == nil {
		t.Fatal("Reload: expected error, got nil")
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if got := handler.saveCount(); got != 1 {
		t.Fatalf("SaveConfig calls = %d, want 1", got)
	}
}

type explainKeyRegressionConfig struct {
	Structure
	Port   func() int    `cfggo:"port" default:"8080" help:"HTTP listen port"`
	APIKey func() string `cfggo:"api_key" default:"dev-secret" secret:"true" help:"API key"`
}

func TestExplainKeyIncludesFocusedDebugContext(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("PORT", "9090")

	cfg := &explainKeyRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	out := cfg.ExplainKey("port")
	for _, want := range []string{
		"port:",
		"value: 9090",
		"type: int",
		"source: env",
		"source_chain: default->env",
		"env: PORT",
		"default: 8080",
		"help: HTTP listen port",
		"status: ok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainKey(port) = %q, missing %q", out, want)
		}
	}
}

func TestExplainKeyMasksSecretsAndReportsUnknownKey(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &explainKeyRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	out := cfg.ExplainKey("api_key")
	if !strings.Contains(out, "value: ****") || !strings.Contains(out, "default: ****") {
		t.Fatalf("ExplainKey(api_key) = %q, want masked value and default", out)
	}
	if strings.Contains(out, "dev-secret") {
		t.Fatalf("ExplainKey(api_key) leaked secret default: %q", out)
	}

	if got := cfg.ExplainKey("missing"); !strings.Contains(got, "missing: <unknown key>") {
		t.Fatalf("ExplainKey(missing) = %q, want unknown-key explanation", got)
	}
}

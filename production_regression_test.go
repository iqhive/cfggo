package cfggo

import (
	"encoding/json"
	"os"
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

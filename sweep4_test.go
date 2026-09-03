package cfggo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iqhive/cfggo/cfglogger"
)

// 1. keys containing "-" are readable from the environment
type dashKeyConfig struct {
	Structure
	DBHost func() string `cfggo:"db-host"`
	Auth   struct {
		APIKey func() string `cfggo:"api-key"`
	} `cfggo:"auth"`
}

func TestDashKeysMapToUnderscoreEnvNames(t *testing.T) {
	setProcessArgs(t)
	t.Setenv("DB_HOST", "from-env")
	t.Setenv("AUTH_API_KEY", "key-from-env")
	cfg := &dashKeyConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.DBHost(); got != "from-env" {
		t.Errorf("DBHost() = %q, want the DB_HOST value", got)
	}
	if got := cfg.Auth.APIKey(); got != "key-from-env" {
		t.Errorf("Auth.APIKey() = %q, want the AUTH_API_KEY value", got)
	}
	for _, k := range cfg.DiagnoseData().Keys {
		if strings.Contains(k.EnvVar, "-") {
			t.Errorf("DiagnoseData reports env var %q for %q; a shell cannot export a name with a dash", k.EnvVar, k.Key)
		}
	}
}

// 3. duplicate flat and nested keys
type dupKeyConfig struct {
	Structure
	DB struct {
		Host func() string `cfggo:"host" default:"def"`
	} `cfggo:"db"`
}

func TestDuplicateFlatAndNestedKeysAreRejected(t *testing.T) {
	setProcessArgs(t)
	file := writeConfigFile(t, `{"db.host":"flat","db":{"host":"nested"}}`)
	cfg := &dupKeyConfig{}
	err := cfg.Init(cfg, WithFileConfig(file), WithoutFlags(), WithoutEnv())
	if !errors.Is(err, ErrSource) || !strings.Contains(err.Error(), `db.host`) {
		t.Fatalf("Init error = %v, want ErrSource naming the duplicate key db.host", err)
	}
	if got := cfg.DB.Host(); got != "def" {
		t.Fatalf("DB.Host() after a rejected load = %q, want the default", got)
	}
}

func TestDuplicateKeysUnderLenientLoadAreDeterministic(t *testing.T) {
	setProcessArgs(t)
	file := writeConfigFile(t, `{"db.host":"flat","db":{"host":"nested"}}`)
	seen := map[string]bool{}
	for i := 0; i < 30; i++ {
		var logs bytes.Buffer
		cfg := &dupKeyConfig{}
		if err := cfg.Init(cfg, WithFileConfig(file), WithoutFlags(), WithoutEnv(), WithLenientLoad(),
			WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs))); err != nil {
			t.Fatalf("Init: %v", err)
		}
		seen[cfg.DB.Host()] = true
		if i == 0 && !strings.Contains(logs.String(), "duplicate") {
			t.Errorf("lenient load did not warn about the duplicate key:\n%s", logs.String())
		}
	}
	if len(seen) != 1 || !seen["nested"] {
		t.Fatalf("winners across loads = %v, want only \"nested\" (first in key order)", seen)
	}
}

// 5. embedding *cfggo.Structure by pointer
type pointerEmbedConfig struct {
	*Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestNilPointerEmbeddedStructureIsAllocatedByInit(t *testing.T) {
	setProcessArgs(t)
	cfg := &pointerEmbedConfig{}
	if err := Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("cfggo.Init: %v", err)
	}
	if cfg.Structure == nil {
		t.Fatal("embedded *Structure still nil after Init")
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want 8080", got)
	}
	if _, ok := cfg.Get("Structure"); ok {
		t.Fatal("the embedded pointer field was registered as a configuration key")
	}
}

func TestNilStructureReceiverReturnsError(t *testing.T) {
	var s *Structure
	err := s.Init(&pointerEmbedConfig{}, WithoutFlags())
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("Init on a nil *Structure = %v, want a descriptive error, not a panic", err)
	}
	if err := s.InitSelf(WithoutFlags()); err == nil {
		t.Fatal("InitSelf on a nil *Structure succeeded")
	}
}

// hardening: the auto-save goroutine of a reverted Init exits
func TestRevertedInitStopsAutoSaveGoroutine(t *testing.T) {
	setProcessArgs(t)
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(), WithAutoSave(ctx)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if runtime.NumGoroutine() <= before {
		t.Skip("could not observe the auto-save goroutine")
	}
	cfg.revertInit()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		t.Fatalf("goroutines after revert = %d, want back to %d: the auto-save goroutine leaked", got, before)
	}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init after revert: %v", err)
	}
	_ = os.Args
}

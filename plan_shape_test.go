package cfggo

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/iqhive/cfggo/cfglogger"
)

type recursiveNode struct {
	Name func() string  `cfggo:"name"`
	Next *recursiveNode `cfggo:"next"`
}

type recursiveConfig struct {
	Structure
	Root *recursiveNode `cfggo:"root"`
}

type pingGroup struct {
	Pong *pongGroup `cfggo:"pong"`
}

type pongGroup struct {
	Ping *pingGroup `cfggo:"ping"`
}

type mutualRecursionConfig struct {
	Structure
	Ping pingGroup `cfggo:"ping"`
}

func initWithTimeout(t *testing.T, run func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Init did not return: the struct walk did not terminate")
		return nil
	}
}

func TestInitRejectsRecursiveStructTypes(t *testing.T) {
	for name, run := range map[string]func() error{
		"self-referential pointer": func() error {
			cfg := &recursiveConfig{}
			return cfg.Init(cfg, WithoutFlags(), WithoutEnv())
		},
		"mutually recursive groups": func() error {
			cfg := &mutualRecursionConfig{}
			return cfg.Init(cfg, WithoutFlags(), WithoutEnv())
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := initWithTimeout(t, run)
			if err == nil || !strings.Contains(err.Error(), "recursive configuration struct") {
				t.Fatalf("Init error = %v, want a recursive-struct error", err)
			}
		})
	}
}

type addrGroup struct {
	Host func() string `cfggo:"host" default:"localhost"`
}

type reusedGroupConfig struct {
	Structure
	Primary addrGroup `cfggo:"primary"`
	Replica addrGroup `cfggo:"replica"`
	Nested  struct {
		Inner addrGroup `cfggo:"inner"`
	} `cfggo:"nested"`
}

func TestSameGroupTypeReusedAsSiblingsIsNotRecursive(t *testing.T) {
	cfg := &reusedGroupConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, key := range []string{"primary.host", "replica.host", "nested.inner.host"} {
		if v, ok := cfg.Get(key); !ok || v != "localhost" {
			t.Errorf("Get(%q) = %v, %v", key, v, ok)
		}
	}
	if cfg.Nested.Inner.Host() != "localhost" {
		t.Errorf("Nested.Inner.Host() = %q", cfg.Nested.Inner.Host())
	}
}

type unexportedAccessorConfig struct {
	Structure
	port    func() int    `cfggo:"port" default:"80"` // deliberately unexported for the test
	Public  func() string `cfggo:"public" default:"ok"`
	counter int           // plain unexported state must be ignored silently
}

func TestUnexportedAccessorFieldIsIgnoredWithWarning(t *testing.T) {
	var logs bytes.Buffer
	cfg := &unexportedAccessorConfig{counter: 7}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv(),
		WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs))); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, ok := cfg.Get("port"); ok {
		t.Error("Get(\"port\") found a key for an unexported field that can never be wired")
	}
	if cfg.port != nil {
		t.Error("unexported accessor field was set; reflection should not be able to write it")
	}
	if got := cfg.Public(); got != "ok" {
		t.Errorf("Public() = %q, want %q", got, "ok")
	}
	if ref := cfg.ConfigReference(); strings.Contains(ref, "\nport ") {
		t.Errorf("ConfigReference lists the unwired key:\n%s", ref)
	}
	if !strings.Contains(logs.String(), "unexported") || !strings.Contains(logs.String(), "key=port") {
		t.Errorf("expected a warning naming the unexported field, got:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "counter") {
		t.Errorf("plain unexported state should not be reported:\n%s", logs.String())
	}
	if cfg.counter != 7 {
		t.Errorf("counter = %d, want plain unexported state left untouched", cfg.counter)
	}
}

type baseSettings struct {
	Host func() string `cfggo:"host" default:"localhost"`
}

type embedsUnexportedConfig struct {
	Structure
	baseSettings `cfggo:"base"`
}

func TestUnexportedEmbeddedStructFieldsStillWork(t *testing.T) {
	cfg := &embedsUnexportedConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "localhost" {
		t.Fatalf("Host() through unexported embedded struct = %q", got)
	}
	if v, ok := cfg.Get("base.host"); !ok || v != "localhost" {
		t.Fatalf("Get(base.host) = %v, %v", v, ok)
	}
}

package cfggo

import (
	"os"
	"testing"
)

// envPrefixConfig is used to verify environment option behavior.
type envPrefixConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"1"`
}

func TestWithEnvConfigUsesUnprefixedAutomaticEnvLayer(t *testing.T) {
	// Isolate from os.Args pollution leaked by other tests: these tests
	// auto-parse os.Args, and a stray flag would abort the whole binary
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	t.Setenv("MYAPP_PORT", "9999")
	t.Setenv("PORT", "4321")

	cfg := &envPrefixConfig{}
	if err := cfg.Init(cfg, WithEnvConfig(), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.Port(); got != 4321 {
		t.Fatalf("Port() = %d, want 4321 from unprefixed PORT", got)
	}
	if got := cfg.DiagnoseData().Keys[0].EnvVar; got != "PORT" {
		t.Fatalf("EnvVar = %q, want PORT", got)
	}
}

func TestWithEnvPrefixUsesAutomaticEnvLayer(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	t.Setenv("MYAPP_PORT", "7777")
	t.Setenv("PORT", "4321")

	cfg := &envPrefixConfig{}
	if err := cfg.Init(cfg, WithEnvPrefix("MYAPP_"), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.Port(); got != 7777 {
		t.Fatalf("Port() = %d, want 7777 from prefixed env", got)
	}
	if src, _ := cfg.Source("port"); src != SourceEnv {
		t.Fatalf("Source(port) = %s, want env", src)
	}
	if got := cfg.DiagnoseData().Keys[0].EnvVar; got != "MYAPP_PORT" {
		t.Fatalf("EnvVar = %q, want MYAPP_PORT", got)
	}
}

func TestWithEnvPrefixAndWithEnvConfigFollowOptionOrder(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options []Option
		want    int
	}{
		{
			name:    "prefix after env config",
			options: []Option{WithEnvConfig(), WithEnvPrefix("MYAPP_"), WithoutFlags()},
			want:    7777,
		},
		{
			name:    "env config after prefix",
			options: []Option{WithEnvPrefix("MYAPP_"), WithEnvConfig(), WithoutFlags()},
			want:    4321,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs := os.Args
			os.Args = []string{"cmd"}
			defer func() { os.Args = oldArgs }()

			t.Setenv("MYAPP_PORT", "7777")
			t.Setenv("PORT", "4321")

			cfg := &envPrefixConfig{}
			if err := cfg.Init(cfg, tt.options...); err != nil {
				t.Fatalf("Init: %v", err)
			}

			if got := cfg.Port(); got != tt.want {
				t.Fatalf("Port() = %d, want %d", got, tt.want)
			}
		})
	}
}

type failedInitConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"notanumber"`
	Host func() string
}

func TestFailedInitRestoresPreInitState(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	cfg := &failedInitConfig{}
	err := cfg.Init(cfg, WithoutFlags())
	if err == nil {
		t.Fatal("Init should have failed with invalid default tag")
	}

	if cfg.parent != nil {
		t.Errorf("cfg.parent = %v, want nil after failed Init", cfg.parent)
	}
	if cfg.plan != nil {
		t.Errorf("cfg.plan = %v, want nil after failed Init", cfg.plan)
	}
	if cfg.initialized.Load() {
		t.Errorf("cfg.initialized = true, want false after failed Init")
	}
	if len(cfg.configData) != 0 {
		t.Errorf("len(cfg.configData) = %d, want 0 after failed Init", len(cfg.configData))
	}
	if len(cfg.defaultData) != 0 {
		t.Errorf("len(cfg.defaultData) = %d, want 0 after failed Init", len(cfg.defaultData))
	}
	if len(cfg.provenance) != 0 {
		t.Errorf("len(cfg.provenance) = %d, want 0 after failed Init", len(cfg.provenance))
	}
	if cfg.changed {
		t.Errorf("cfg.changed = true, want false after failed Init")
	}
	if cfg.changeVersion != 0 {
		t.Errorf("cfg.changeVersion = %d, want 0 after failed Init", cfg.changeVersion)
	}
}

type lateFailConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestFailedInitAfterPlanResetsToDefaults(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	defer func() { os.Args = oldArgs }()

	t.Setenv("PORT", "not-a-number")

	cfg := &lateFailConfig{}
	err := cfg.Init(cfg, WithoutFlags())
	if err == nil {
		t.Fatal("Init should have failed with invalid env value")
	}

	if cfg.initialized.Load() {
		t.Errorf("cfg.initialized = true, want false after failed Init")
	}
	if got := cfg.Port(); got != 8080 {
		t.Errorf("Port() = %d, want 8080 default", got)
	}
	if src, _ := cfg.Source("port"); src != SourceDefault {
		t.Errorf("Source(port) = %v, want %v", src, SourceDefault)
	}
}

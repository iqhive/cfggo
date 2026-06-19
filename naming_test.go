package cfggo

import (
	"flag"
	"os"
	"testing"
)

type snakeNamingNestedConfig struct {
	HTTPServerURL func() string
}

type snakeNamingConfig struct {
	Structure
	ServerPort func() int
	APIKey     func() string
	JSONName   func() string `json:"json_name"`
	Nested     snakeNamingNestedConfig
}

func TestDefaultFieldNamingPreservesGoFieldNames(t *testing.T) {
	cfg := &snakeNamingConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, key := range []string{"ServerPort", "APIKey", "json_name", "Nested.HTTPServerURL"} {
		if _, ok := cfg.configData[key]; !ok {
			t.Fatalf("configData missing key %q; got %#v", key, cfg.configData)
		}
	}
}

func TestDefaultSnakeCaseFieldNamesGlobal(t *testing.T) {
	old := DefaultSnakeCaseFieldNames
	DefaultSnakeCaseFieldNames = true
	t.Cleanup(func() { DefaultSnakeCaseFieldNames = old })

	cfg := &snakeNamingConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, key := range []string{"server_port", "api_key", "json_name", "nested.http_server_url"} {
		if _, ok := cfg.configData[key]; !ok {
			t.Fatalf("configData missing key %q; got %#v", key, cfg.configData)
		}
	}
}

func TestWithSnakeCaseFieldNamesOverridesGlobalDefault(t *testing.T) {
	old := DefaultSnakeCaseFieldNames
	DefaultSnakeCaseFieldNames = true
	t.Cleanup(func() { DefaultSnakeCaseFieldNames = old })

	cfg := &snakeNamingConfig{}
	if err := cfg.Init(cfg, WithSnakeCaseFieldNames(false), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if _, ok := cfg.configData["ServerPort"]; !ok {
		t.Fatalf("configData missing raw key ServerPort; got %#v", cfg.configData)
	}
	if _, ok := cfg.configData["server_port"]; ok {
		t.Fatalf("configData unexpectedly contains snake key server_port; got %#v", cfg.configData)
	}
}

func TestSnakeCaseFieldNamesDriveEnvAndFlags(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"cmd"}
	t.Cleanup(func() { os.Args = oldArgs })

	t.Setenv("API_KEY", "from-env")
	fs := flag.NewFlagSet("snake", flag.ContinueOnError)

	cfg := &snakeNamingConfig{}
	if err := cfg.Init(cfg, WithSnakeCaseFieldNames(true), WithFlagSet(fs)); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := cfg.APIKey(); got != "from-env" {
		t.Fatalf("APIKey() = %q, want from-env", got)
	}
	if fs.Lookup("server_port") == nil {
		t.Fatalf("server_port flag was not registered")
	}
	if fs.Lookup("ServerPort") != nil {
		t.Fatalf("raw ServerPort flag was unexpectedly registered")
	}
}

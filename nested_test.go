package cfggo

import (
	"os"
	"path/filepath"
	"testing"
)

// Database is a nested (non-embedded) config sub-struct.
type Database struct {
	Host   func() string `cfggo:"host" default:"localhost" help:"database host"`
	Port   func() int    `cfggo:"port" default:"5432"`
	Secret func() string `cfggo:"-"`
}

// Server is another nested level to exercise deeper recursion.
type Server struct {
	Listen func() string `cfggo:"listen" default:":8080"`
	DB     Database      `cfggo:"db"`
}

type NestedConfig struct {
	Structure
	Name   func() string `cfggo:"name" default:"svc"`
	Server Server        `cfggo:"server"`
}

// PtrNestedConfig exercises pointer sub-structs at multiple depths
type PtrNestedConfig struct {
	Structure
	Name   func() string `cfggo:"name" default:"svc"`
	Server *Server       `cfggo:"server"`
}

func TestPtrNestedDefaultsAndAccessors(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	// Server (and its nested *Database via the value field) is left nil
	// Init must allocate it so the accessors are usable
	cfg := &PtrNestedConfig{}
	cfg.Init(cfg)

	if cfg.Server == nil {
		t.Fatal("Server pointer sub-struct was not allocated by Init")
	}
	if got := cfg.Server.Listen(); got != ":8080" {
		t.Errorf("Server.Listen = %q, want %q", got, ":8080")
	}
	if got := cfg.Server.DB.Host(); got != "localhost" {
		t.Errorf("Server.DB.Host = %q, want %q", got, "localhost")
	}
	if got := cfg.Server.DB.Port(); got != 5432 {
		t.Errorf("Server.DB.Port = %d, want %d", got, 5432)
	}
}

func TestPtrNestedFileEnvFlagHelpReload(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	os.Setenv("SERVER_LISTEN", ":9090")
	defer os.Unsetenv("SERVER_LISTEN")

	dir := t.TempDir()
	file := filepath.Join(dir, "ptr_nested.json")
	if err := os.WriteFile(file, []byte(`{"server":{"db":{"host":"db1","port":1111}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &PtrNestedConfig{}
	cfg.Init(cfg, WithFileConfig(file))

	// File value through a pointer sub-struct
	if got := cfg.Server.DB.Host(); got != "db1" {
		t.Errorf("Server.DB.Host = %q, want %q", got, "db1")
	}
	// Env value
	if got := cfg.Server.Listen(); got != ":9090" {
		t.Errorf("Server.Listen = %q, want %q", got, ":9090")
	}
	// Nested help tag resolves through the pointer
	if got := cfg.GetHelpTag("server.db.host"); got != "database host" {
		t.Errorf("GetHelpTag(server.db.host) = %q, want %q", got, "database host")
	}

	// Hot reload is reflected through the same accessors
	if err := os.WriteFile(file, []byte(`{"server":{"db":{"host":"db2","port":2222}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	if got := cfg.Server.DB.Host(); got != "db2" {
		t.Errorf("after reload Server.DB.Host = %q, want %q", got, "db2")
	}
	if got := cfg.Server.DB.Port(); got != 2222 {
		t.Errorf("after reload Server.DB.Port = %d, want %d", got, 2222)
	}
}

func TestPtrNestedPreallocatedOverride(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	// A caller-supplied pointer with a function override must be respected
	// Function value wins over the tag default, just like value sub-structs
	cfg := &PtrNestedConfig{
		Server: &Server{
			Listen: DefaultValue(":7000"),
		},
	}
	cfg.Init(cfg)

	if got := cfg.Server.Listen(); got != ":7000" {
		t.Errorf("Server.Listen = %q, want %q", got, ":7000")
	}
	// Unset nested field still gets its default
	if got := cfg.Server.DB.Port(); got != 5432 {
		t.Errorf("Server.DB.Port = %d, want %d", got, 5432)
	}
}

func TestNestedStructDefaultsAndAccessors(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	cfg := &NestedConfig{}
	cfg.Init(cfg)

	if got := cfg.Name(); got != "svc" {
		t.Errorf("Name = %q, want %q", got, "svc")
	}
	if got := cfg.Server.Listen(); got != ":8080" {
		t.Errorf("Server.Listen = %q, want %q", got, ":8080")
	}
	if got := cfg.Server.DB.Host(); got != "localhost" {
		t.Errorf("Server.DB.Host = %q, want %q", got, "localhost")
	}
	if got := cfg.Server.DB.Port(); got != 5432 {
		t.Errorf("Server.DB.Port = %d, want %d", got, 5432)
	}
}

func TestNestedStructFileOverrideAndReload(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	dir := t.TempDir()
	file := filepath.Join(dir, "nested.json")
	if err := os.WriteFile(file, []byte(`{"server":{"db":{"host":"db1","port":1111}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &NestedConfig{}
	cfg.Init(cfg, WithFileConfig(file))

	if got := cfg.Server.DB.Host(); got != "db1" {
		t.Errorf("Server.DB.Host = %q, want %q", got, "db1")
	}
	if got := cfg.Server.DB.Port(); got != 1111 {
		t.Errorf("Server.DB.Port = %d, want %d", got, 1111)
	}
	// Listen had no file value, so the default must still apply.
	if got := cfg.Server.Listen(); got != ":8080" {
		t.Errorf("Server.Listen = %q, want %q", got, ":8080")
	}

	// Hot reload should be reflected through the same accessor closures.
	if err := os.WriteFile(file, []byte(`{"server":{"db":{"host":"db2","port":2222}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	if got := cfg.Server.DB.Host(); got != "db2" {
		t.Errorf("after reload Server.DB.Host = %q, want %q", got, "db2")
	}
	if got := cfg.Server.DB.Port(); got != 2222 {
		t.Errorf("after reload Server.DB.Port = %d, want %d", got, 2222)
	}
}

func TestNestedStructEnvOverride(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	os.Setenv("SERVER_DB_HOST", "env-db")
	defer os.Unsetenv("SERVER_DB_HOST")

	cfg := &NestedConfig{}
	cfg.Init(cfg)

	if got := cfg.Server.DB.Host(); got != "env-db" {
		t.Errorf("Server.DB.Host = %q, want %q", got, "env-db")
	}
}

func TestNestedStructHelpTag(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	cfg := &NestedConfig{}
	cfg.Init(cfg)

	if got := cfg.GetHelpTag("server.db.host"); got != "database host" {
		t.Errorf("GetHelpTag(server.db.host) = %q, want %q", got, "database host")
	}
	// A nested key without a help tag returns an empty string.
	if got := cfg.GetHelpTag("server.db.port"); got != "" {
		t.Errorf("GetHelpTag(server.db.port) = %q, want empty", got)
	}
}

func TestNestedIgnoredFieldNotManaged(t *testing.T) {
	SetupNewFlags()
	defer RestoreFlagValues()
	os.Args = []string{"cmd"}

	cfg := &NestedConfig{}
	cfg.Init(cfg)

	// A field tagged cfggo:"-" is never added to the managed config map,
	// at any nesting depth
	if _, ok := cfg.Get("server.db.Secret"); ok {
		t.Errorf("ignored field server.db.Secret should not be present in config map")
	}
	if _, ok := cfg.Get("server.db.-"); ok {
		t.Errorf("ignored field should not be present under a %q key", "-")
	}
}

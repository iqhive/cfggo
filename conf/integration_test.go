package conf_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/conf"
)

type databaseConfig struct {
	Host func() string `cfggo:"host" default:"localhost"`
	Pool struct {
		Size func() int `cfggo:"size" default:"5"`
	} `cfggo:"pool"`
}

type integrationConfig struct {
	cfggo.Structure
	Name     func() string        `cfggo:"name" default:"default"`
	Port     func() uint16        `cfggo:"port" default:"80"`
	Debug    func() bool          `cfggo:"debug" default:"false"`
	Timeout  func() time.Duration `cfggo:"timeout" default:"1s"`
	ID       func() uint64        `cfggo:"id" default:"0"`
	Database databaseConfig       `cfggo:"database"`
}

func TestCfggoIntegrationAndReloadRollback(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.conf")
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("name=api # comment\nport=8080\nid=18446744073709551615\n[database]\nhost=db.internal\n[database.pool]\nsize=20\n[global]\ndebug=yes\ntimeout=5s\n")
	handler, err := conf.NewFileHandler(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	config := &integrationConfig{}
	if err := config.Init(config, cfggo.WithConfigHandler(handler), cfggo.WithStrictKeys(), cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if config.Name() != "api" || config.Port() != 8080 || !config.Debug() || config.Timeout() != 5*time.Second ||
		config.ID() != ^uint64(0) || config.Database.Host() != "db.internal" || config.Database.Pool.Size() != 20 {
		t.Fatalf("unexpected loaded config: %s", config.String())
	}

	write("name=changed\n[broken\n")
	if err := config.ReloadConfig(); err == nil || !errors.Is(err, cfggo.ErrSource) || !errors.Is(err, conf.ErrSyntax) {
		t.Fatalf("ReloadConfig error = %v", err)
	}
	if config.Name() != "api" || config.Port() != 8080 {
		t.Fatalf("failed reload changed live values: %s", config.String())
	}
}

type serviceConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"80"`
}

type rootConfig struct {
	cfggo.Structure
	Region func() string `cfggo:"region" default:"local"`
}

func TestGroupIntegration(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "group.conf")
	if err := os.WriteFile(filename, []byte("region=west\n[api]\nport=8080\n[worker]\nport=9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := conf.NewFileHandler(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	root, api, worker := &rootConfig{}, &serviceConfig{}, &serviceConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags())
	group.Register("", root, cfggo.WithoutEnv())
	group.Register("api", api, cfggo.WithoutEnv())
	group.Register("worker", worker, cfggo.WithoutEnv())
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if root.Region() != "west" || api.Port() != 8080 || worker.Port() != 9090 {
		t.Fatalf("group values = %q, %d, %d", root.Region(), api.Port(), worker.Port())
	}
}

func TestConfNumericTextLoadsIntoStringFields(t *testing.T) {
	type textFields struct {
		cfggo.Structure
		Region  func() string      `cfggo:"region"`
		Version func() string      `cfggo:"version"`
		Flag    func() string      `cfggo:"flag"`
		Any     func() interface{} `cfggo:"any"`
	}
	filename := filepath.Join(t.TempDir(), "text.conf")
	if err := os.WriteFile(filename, []byte("region = 12\nversion = \"1.0\"\nflag = true\nany = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := conf.NewFileHandler(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &textFields{}
	if err := cfg.Init(cfg, cfggo.WithConfigHandler(handler), cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if cfg.Region() != "12" || cfg.Version() != "1.0" || cfg.Flag() != "true" {
		t.Fatalf("text fields = %q %q %q, want literal text", cfg.Region(), cfg.Version(), cfg.Flag())
	}
	if got := cfg.Any(); got != float64(42) {
		t.Fatalf("Any() = %#v, want the typed number 42", got)
	}
}

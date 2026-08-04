package cfggo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type groupedConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"80"`
	Name func() string `cfggo:"name" default:"default"`
}

func TestGroupFileEnvAndFlags(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080},"billing":{"name":"file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYAPP_AUTH_PORT", "9090")
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test", "--billing.name=flag"}

	auth := &groupedConfig{}
	billing := &groupedConfig{}
	group := NewGroup(GroupWithFileConfig(file), GroupWithEnvPrefix("MYAPP_"))
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatal(err)
	}
	if got := auth.Port(); got != 9090 {
		t.Fatalf("auth.Port() = %d, want 9090", got)
	}
	if got := billing.Name(); got != "flag" {
		t.Fatalf("billing.Name() = %q, want flag", got)
	}
	if group.flagSet.Lookup("auth.port") == nil {
		t.Fatal("auth.port flag was not registered")
	}
}

func TestGroupRootCollisions(t *testing.T) {
	a, b := &groupedConfig{}, &groupedConfig{}
	group := NewGroup()
	group.Register("", a)
	group.Register("", b)
	if err := group.Init(); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("Init() error = %v, want collision", err)
	}
}

func TestGroupSaveAndReload(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080},"billing":{"port":9090}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, billing := &groupedConfig{}, &groupedConfig{}
	group := NewGroup(GroupWithFileConfig(file))
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatal(err)
	}
	if err := group.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"port":8080`) {
		t.Fatalf("saved file = %s", data)
	}
	if err := os.WriteFile(file, []byte(`{"auth":{"port":7070},"billing":{"port":6060}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := group.Reload(); err != nil {
		t.Fatal(err)
	}
	if auth.Port() != 7070 || billing.Port() != 6060 {
		t.Fatalf("reloaded values = %d, %d", auth.Port(), billing.Port())
	}
}

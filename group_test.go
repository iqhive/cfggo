package cfggo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/validcfg"
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

func TestGroupMemberOptionsValidation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	group := NewGroup(GroupWithFileConfig(file))
	group.Register("auth", &groupedConfig{}, WithValidation("port", validcfg.Range(1, 65535)))
	if err := group.Init(); err == nil {
		t.Fatal("Init() succeeded with invalid per-member port")
	}
}

func TestGroupProvenance(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080,"name":"file"},"billing":{"name":"file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYAPP_BILLING_NAME", "env")
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test", "--auth.name=flag"}

	auth, billing := &groupedConfig{}, &groupedConfig{}
	group := NewGroup(GroupWithFileConfig(file), GroupWithEnvPrefix("MYAPP_"))
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatal(err)
	}
	if source, ok := auth.Source("port"); !ok || source != SourceFile {
		t.Fatalf("auth port source = %v, %t; want file", source, ok)
	}
	if source, ok := billing.Source("name"); !ok || source != SourceEnv {
		t.Fatalf("billing name source = %v, %t; want env", source, ok)
	}
	if source, ok := auth.Source("name"); !ok || source != SourceFlag {
		t.Fatalf("auth name source = %v, %t; want flag", source, ok)
	}
	explain := group.Explain()
	for _, want := range []string{"auth:", "billing:", "file", "env", "flag"} {
		if !strings.Contains(explain, want) {
			t.Fatalf("group Explain() missing %q: %s", want, explain)
		}
	}
}

func TestGroupReloadRollback(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := &groupedConfig{}
	group := NewGroup(GroupWithFileConfig(file))
	group.Register("auth", auth, WithValidation("port", validcfg.Range(1, 65535)))
	if err := group.Init(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"auth":{"port":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := group.Reload(); err == nil {
		t.Fatal("Reload() succeeded with invalid port")
	}
	if got := auth.Port(); got != 8080 {
		t.Fatalf("auth.Port() after failed reload = %d, want 8080", got)
	}
}

func TestGroupSaveIfChangedNoOp(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"auth":{"port":8080}}`)
	if err := os.WriteFile(file, original, 0o600); err != nil {
		t.Fatal(err)
	}
	group := NewGroup(GroupWithFileConfig(file))
	group.Register("auth", &groupedConfig{})
	if err := group.Init(); err != nil {
		t.Fatal(err)
	}
	if err := group.SaveIfChanged(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("SaveIfChanged() rewrote unchanged file: %s", got)
	}
}

func TestGroupRootKeyNamespaceCollision(t *testing.T) {
	group := NewGroup()
	group.Register("", &groupedConfig{})
	group.Register("port", &groupedConfig{})
	if err := group.Init(); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("Init() error = %v, want root/namespace collision", err)
	}
}

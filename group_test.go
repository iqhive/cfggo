package cfggo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type authConfig struct {
	Structure
	Port   func() int    `cfggo:"port" default:"8080" help:"listen port"`
	DBHost func() string `cfggo:"db_host"`
}

type billingConfig struct {
	Structure
	Port   func() int    `cfggo:"port" default:"9000"`
	DBHost func() string `cfggo:"db_host"`
}

func groupTestArgs(args ...string) func() {
	old := os.Args
	os.Args = append([]string{"group-test"}, args...)
	return func() { os.Args = old }
}

func TestGroupFileEnvFlagNamespace(t *testing.T) {
	restore := groupTestArgs("--auth.port=9090", "--billing.port=9091")
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	combined := `{"auth":{"port":8080},"billing":{"port":9001}}`
	if err := os.WriteFile(file, []byte(combined), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("MYAPP_AUTH_PORT", "7070")
	t.Setenv("MYAPP_BILLING_PORT", "7071")

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup(
		GroupWithFileConfig(file),
		GroupWithEnvPrefix("MYAPP_"),
	)
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Precedence: flags override env, env overrides file, file overrides default.
	if got := auth.Port(); got != 9090 {
		t.Errorf("auth.Port() = %d, want 9090 (from flag)", got)
	}
	if got := billing.Port(); got != 9091 {
		t.Errorf("billing.Port() = %d, want 9091 (from flag)", got)
	}

	// Non-overridden keys come from the file.
	if got := auth.DBHost(); got != "" {
		t.Errorf("auth.DBHost() = %q, want empty", got)
	}
}

func TestGroupEnvAndFileNamespace(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	combined := `{"auth":{"db_host":"auth.db"},"billing":{"db_host":"billing.db"}}`
	if err := os.WriteFile(file, []byte(combined), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("MYAPP_AUTH_PORT", "6060")
	t.Setenv("MYAPP_BILLING_PORT", "6061")

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup(
		GroupWithFileConfig(file),
		GroupWithEnvPrefix("MYAPP_"),
	)
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := auth.Port(); got != 6060 {
		t.Errorf("auth.Port() = %d, want 6060 (from env)", got)
	}
	if got := billing.Port(); got != 6061 {
		t.Errorf("billing.Port() = %d, want 6061 (from env)", got)
	}
	if got := auth.DBHost(); got != "auth.db" {
		t.Errorf("auth.DBHost() = %q, want auth.db (from file)", got)
	}
	if got := billing.DBHost(); got != "billing.db" {
		t.Errorf("billing.DBHost() = %q, want billing.db (from file)", got)
	}
}

func TestGroupRootNamespaceCollision(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup()
	if err := group.Register("", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}

	err := group.Init()
	if err == nil {
		t.Fatal("Init expected root namespace collision error, got nil")
	}
	if !errors.Is(err, ErrUnknownKey) {
		t.Errorf("expected ErrUnknownKey, got %v", err)
	}
}

func TestGroupValidationPerMember(t *testing.T) {
	restore := groupTestArgs("--auth.port=70000")
	defer restore()

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup()
	if err := group.Register("auth", auth, WithValidation("port", Range(1, 65535))); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}

	err := group.Init()
	if err == nil {
		t.Fatal("Init expected validation error, got nil")
	}
	// The standard flag package does not wrap a Value.Set validation error, so
	// we surface the failure as an invalid-argument parse error.
	if ErrorCode(err) != ErrCodeInvalidArgument {
		t.Errorf("expected error code %d, got %d: %v", ErrCodeInvalidArgument, ErrorCode(err), err)
	}
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("expected validation message, got %v", err)
	}
}

func TestGroupSaveRoundTrip(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	auth := &authConfig{Port: DefaultValue(9090)}
	billing := &billingConfig{Port: DefaultValue(9091), DBHost: DefaultValue("billing.db")}

	group := NewGroup(GroupWithFileConfig(file))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := group.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var combined map[string]map[string]interface{}
	if err := json.Unmarshal(data, &combined); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got, ok := combined["auth"]["port"]; !ok || got != float64(9090) {
		t.Errorf("auth.port = %v, want 9090", got)
	}
	if got, ok := combined["billing"]["port"]; !ok || got != float64(9091) {
		t.Errorf("billing.port = %v, want 9091", got)
	}
	if got, ok := combined["billing"]["db_host"]; !ok || got != "billing.db" {
		t.Errorf("billing.db_host = %v, want billing.db", got)
	}
}

func TestGroupSaveIfChangedOnlyWhenChanged(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup(GroupWithFileConfig(file))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// After Init defaults are loaded; changed is set when values are applied,
	// so the first SaveIfChanged should write the defaults out.
	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("first SaveIfChanged: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	size1 := info.Size()

	// Second SaveIfChanged with no further changes is a no-op.
	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("second SaveIfChanged: %v", err)
	}
	info, err = os.Stat(file)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.Size() != size1 {
		t.Errorf("second SaveIfChanged wrote even though nothing changed")
	}
}

func TestGroupReloadDispatch(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080}}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	auth := &authConfig{}
	billing := &billingConfig{}

	group := NewGroup(GroupWithFileConfig(file))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Register("billing", billing); err != nil {
		t.Fatalf("Register billing: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := auth.Port(); got != 8080 {
		t.Errorf("auth.Port() before reload = %d, want 8080", got)
	}

	// Update the combined file and reload.
	if err := os.WriteFile(file, []byte(`{"auth":{"port":9090},"billing":{"port":9005}}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := group.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if got := auth.Port(); got != 9090 {
		t.Errorf("auth.Port() after reload = %d, want 9090", got)
	}
	if got := billing.Port(); got != 9005 {
		t.Errorf("billing.Port() after reload = %d, want 9005", got)
	}
}

func TestGroupReloadRollbackOnBadFile(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080}}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	auth := &authConfig{}

	group := NewGroup(GroupWithFileConfig(file))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Corrupt the file and reload; member should keep previous values.
	if err := os.WriteFile(file, []byte(`not json`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := group.Reload()
	if err == nil {
		t.Fatal("Reload expected error for malformed JSON, got nil")
	}

	if got := auth.Port(); got != 8080 {
		t.Errorf("auth.Port() after failed reload = %d, want 8080", got)
	}
}

func TestGroupProvenanceThroughGroup(t *testing.T) {
	restore := groupTestArgs("--auth.port=1234")
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	if err := os.WriteFile(file, []byte(`{"auth":{"port":8080}}`), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	t.Setenv("MYAPP_AUTH_PORT", "9090")

	auth := &authConfig{}

	group := NewGroup(
		GroupWithFileConfig(file),
		GroupWithEnvPrefix("MYAPP_"),
	)
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := auth.Port(); got != 1234 {
		t.Errorf("auth.Port() = %d, want 1234", got)
	}

	src, ok := auth.Source("port")
	if !ok || src != SourceFlag {
		t.Errorf("auth.Source(port) = %v %v, want flag", src, ok)
	}

	chain := auth.SourceChain("port")
	if len(chain) < 3 {
		t.Fatalf("auth.SourceChain(port) = %v, want at least default->file->env->flag", chain)
	}
	if chain[0] != SourceDefault {
		t.Errorf("chain[0] = %v, want default", chain[0])
	}
	if chain[len(chain)-1] != SourceFlag {
		t.Errorf("chain[last] = %v, want flag", chain[len(chain)-1])
	}
}

func TestGroupDuplicateNamespace(t *testing.T) {
	auth := &authConfig{}

	group := NewGroup()
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := group.Register("auth", &billingConfig{}); err == nil {
		t.Fatal("second Register with duplicate namespace expected error, got nil")
	}
}

func TestGroupMissingRequiredFile(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	auth := &authConfig{}

	group := NewGroup(GroupWithFileConfig("/does/not/exist.json"))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := group.Init(); err == nil {
		t.Fatal("Init with missing required file expected error, got nil")
	}
}

func TestGroupDefaultMissingFile(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	auth := &authConfig{}

	group := NewGroup(GroupWithDefaultFileConfig("/does/not/exist.json"))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init with missing default file: %v", err)
	}
	if got := auth.Port(); got != 8080 {
		t.Errorf("auth.Port() = %d, want default 8080", got)
	}
}

func TestGroupCombinedWithRootMember(t *testing.T) {
	restore := groupTestArgs()
	defer restore()

	dir := t.TempDir()
	file := filepath.Join(dir, "combined.json")
	combined := `{"port":1000,"auth":{"port":2000}}`
	if err := os.WriteFile(file, []byte(combined), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	auth := &authConfig{}

	group := NewGroup(GroupWithFileConfig(file))
	if err := group.Register("auth", auth); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := group.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// The "auth" object should be consumed by the auth namespace; the top-level
	// "port" key should be ignored because there is no root member that defines
	// it, and auth should not see it as an unknown key.
	if got := auth.Port(); got != 2000 {
		t.Errorf("auth.Port() = %d, want 2000", got)
	}
}

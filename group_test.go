package cfggo_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/validcfg"
)

type authConfig struct {
	cfggo.Structure

	Port   func() int    `cfggo:"port"    default:"8080" help:"listen port"`
	DBHost func() string `cfggo:"db_host" default:"localhost"`
}

type billingConfig struct {
	cfggo.Structure

	Port    func() int    `cfggo:"port"    default:"9090" help:"listen port"`
	Gateway func() string `cfggo:"gateway" default:"stripe"`
}

type rootConfig struct {
	cfggo.Structure

	LogLevel func() string `cfggo:"log_level" default:"info"`
}

// withArgs replaces os.Args for the duration of a test, since cfggo (like the
// flag package) reads the process arguments.
func withArgs(t *testing.T, args ...string) {
	t.Helper()
	original := os.Args
	os.Args = append([]string{"grouptest"}, args...)
	t.Cleanup(func() { os.Args = original })
}

func writeJSON(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "combined.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestGroupNamespacedFileEnvAndFlags(t *testing.T) {
	path := writeJSON(t, `{"auth":{"port":1111,"db_host":"authdb"},"billing":{"port":2222,"gateway":"adyen"}}`)
	withArgs(t, "--auth.port=3333")
	t.Setenv("MYAPP_BILLING_GATEWAY", "paypal")

	auth := &authConfig{}
	billing := &billingConfig{}

	group := cfggo.NewGroup(
		cfggo.GroupWithFileConfig(path),
		cfggo.GroupWithEnvPrefix("MYAPP_"),
	)
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	if got := auth.Port(); got != 3333 {
		t.Errorf("auth port from flag = %d, want 3333", got)
	}
	if got := auth.DBHost(); got != "authdb" {
		t.Errorf("auth db_host from file = %q, want %q", got, "authdb")
	}
	if got := billing.Port(); got != 2222 {
		t.Errorf("billing port from file = %d, want 2222", got)
	}
	if got := billing.Gateway(); got != "paypal" {
		t.Errorf("billing gateway from env = %q, want %q", got, "paypal")
	}
}

func TestGroupProvenanceThroughGroup(t *testing.T) {
	path := writeJSON(t, `{"auth":{"db_host":"authdb"}}`)
	withArgs(t, "--auth.port=3333")

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	if explained := auth.ExplainKey("db_host"); !strings.Contains(explained, "file") {
		t.Errorf("db_host provenance = %q, want it to mention the file source", explained)
	}
	if explained := auth.ExplainKey("port"); !strings.Contains(explained, "flag") {
		t.Errorf("port provenance = %q, want it to mention the flag source", explained)
	}
}

func TestGroupUnprefixedEnvIsNotRead(t *testing.T) {
	withArgs(t)
	t.Setenv("PORT", "4444")

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithEnvPrefix("MYAPP_"))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 8080 {
		t.Errorf("auth port = %d, want the default 8080 (namespaced env only)", got)
	}
}

func TestGroupPerMemberOptions(t *testing.T) {
	withArgs(t, "--billing.port=70000")

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	group.Register("billing", &billingConfig{}, cfggo.WithValidation("port", validcfg.Range(1, 65535)))

	err := group.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want a validation error for billing.port")
	}
	if !strings.Contains(err.Error(), "billing") {
		t.Errorf("error = %v, want it to name the billing member", err)
	}
}

func TestGroupRootNamespaceAndCollisionDetection(t *testing.T) {
	withArgs(t, "--log_level=debug", "--auth.port=1234")

	root := &rootConfig{}
	auth := &authConfig{}
	group := cfggo.NewGroup()
	group.Register("", root)
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := root.LogLevel(); got != "debug" {
		t.Errorf("root log_level = %q, want %q", got, "debug")
	}
	if got := auth.Port(); got != 1234 {
		t.Errorf("auth port = %d, want 1234", got)
	}

	colliding := cfggo.NewGroup()
	colliding.Register("", &authConfig{})
	colliding.Register("", &billingConfig{})
	err := colliding.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want a root key collision error")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error = %v, want it to name the colliding key", err)
	}
}

func TestGroupRootMemberReadsUnnamespacedFileKeys(t *testing.T) {
	path := writeJSON(t, `{"log_level":"warn","auth":{"port":1111}}`)
	withArgs(t)

	root := &rootConfig{}
	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("", root)
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := root.LogLevel(); got != "warn" {
		t.Errorf("root log_level = %q, want %q", got, "warn")
	}
	if got := auth.Port(); got != 1111 {
		t.Errorf("auth port = %d, want 1111", got)
	}
}

func TestGroupSaveRoundTrip(t *testing.T) {
	path := writeJSON(t, `{"auth":{"port":1111},"billing":{"port":2222}}`)
	withArgs(t)

	auth := &authConfig{}
	billing := &billingConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := auth.Set("port", 4444); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := group.Save(); err != nil {
		t.Fatalf("Group.Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var document map[string]map[string]interface{}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal saved config: %v (%s)", err, data)
	}
	if got := document["auth"]["port"]; got != float64(4444) {
		t.Errorf("saved auth.port = %v, want 4444", got)
	}
	if got := document["billing"]["port"]; got != float64(2222) {
		t.Errorf("saved billing.port = %v, want 2222", got)
	}

	// The saved document must load back through a fresh group unchanged.
	reloadedAuth := &authConfig{}
	reloadedBilling := &billingConfig{}
	fresh := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	fresh.Register("auth", reloadedAuth)
	fresh.Register("billing", reloadedBilling)
	if err := fresh.Init(); err != nil {
		t.Fatalf("Group.Init after save: %v", err)
	}
	if got := reloadedAuth.Port(); got != 4444 {
		t.Errorf("round-tripped auth port = %d, want 4444", got)
	}
	if got := reloadedBilling.Gateway(); got != "stripe" {
		t.Errorf("round-tripped billing gateway = %q, want the default", got)
	}
}

func TestGroupSaveIfChanged(t *testing.T) {
	path := writeJSON(t, `{"auth":{"port":1111}}`)
	withArgs(t)

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("Group.SaveIfChanged: %v", err)
	}
	// A second call must be a no-op now that nothing is dirty.
	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("Group.SaveIfChanged (clean): %v", err)
	}
	if err := auth.Set("port", 5555); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("Group.SaveIfChanged (dirty): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "5555") {
		t.Errorf("saved config = %s, want it to contain the new value", data)
	}
}

func TestGroupReloadDispatchesPerMember(t *testing.T) {
	path := writeJSON(t, `{"auth":{"port":1111},"billing":{"port":2222}}`)
	withArgs(t)

	auth := &authConfig{}
	billing := &billingConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("auth", auth)
	group.Register("billing", billing)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	var changes []string
	auth.OnChange(func(cs []cfggo.Change) {
		for _, c := range cs {
			changes = append(changes, c.Key)
		}
	})

	if err := os.WriteFile(path, []byte(`{"auth":{"port":7777},"billing":{"port":8888}}`), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := group.Reload(); err != nil {
		t.Fatalf("Group.Reload: %v", err)
	}
	if got := auth.Port(); got != 7777 {
		t.Errorf("auth port after reload = %d, want 7777", got)
	}
	if got := billing.Port(); got != 8888 {
		t.Errorf("billing port after reload = %d, want 8888", got)
	}
	if len(changes) == 0 {
		t.Error("no OnChange callback fired for the auth member")
	}
}

func TestGroupReloadRollsBackOnBadDocument(t *testing.T) {
	path := writeJSON(t, `{"auth":{"port":1111}}`)
	withArgs(t)

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"auth":{"port":"not-a-number"}}`), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := group.Reload(); err == nil {
		t.Fatal("Group.Reload succeeded, want an error for the invalid value")
	}
	if got := auth.Port(); got != 1111 {
		t.Errorf("auth port after failed reload = %d, want the previous 1111", got)
	}
}

func TestGroupFlagsAreNamespacedAndSharedSet(t *testing.T) {
	withArgs(t, "--auth.port=1", "--billing.port=2")

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	group.Register("billing", &billingConfig{})
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	fs := group.FlagSet()
	if fs == nil {
		t.Fatal("Group.FlagSet() = nil")
	}
	for _, name := range []string{"auth.port", "auth.db_host", "billing.port", "billing.gateway"} {
		if fs.Lookup(name) == nil {
			t.Errorf("flag %q not registered on the shared flag set", name)
		}
	}
	if fs.Lookup("port") != nil {
		t.Error("unprefixed flag \"port\" registered; namespaced members must not collide")
	}
}

func TestGroupUnknownFlagSuggestsNamespacedKey(t *testing.T) {
	withArgs(t, "--auth.prot=1")

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	err := group.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want an unknown-flag error")
	}
	if !strings.Contains(err.Error(), "auth.port") {
		t.Errorf("error = %v, want a suggestion of auth.port", err)
	}
}

func TestGroupIgnoreUnknownFlags(t *testing.T) {
	withArgs(t, "--not-mine=x", "--auth.port=4242")

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithIgnoreUnknownFlags())
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 4242 {
		t.Errorf("auth port = %d, want 4242", got)
	}
}

func TestGroupWithExternalFlagSet(t *testing.T) {
	withArgs(t)

	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFlagSet(fs))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if fs.Parsed() {
		t.Error("group parsed a caller-supplied flag set; the host owns parsing")
	}
	if err := fs.Parse([]string{"--auth.port=6060"}); err != nil {
		t.Fatalf("host Parse: %v", err)
	}
	if got := auth.Port(); got != 6060 {
		t.Errorf("auth port after host parse = %d, want 6060", got)
	}
}

func TestGroupRegistrationErrors(t *testing.T) {
	withArgs(t)

	notAConfig := cfggo.NewGroup()
	notAConfig.Register("auth", &struct{ Port int }{})
	if err := notAConfig.Init(); err == nil {
		t.Error("Init succeeded for a struct that does not embed cfggo.Structure")
	}

	duplicate := cfggo.NewGroup()
	duplicate.Register("auth", &authConfig{})
	duplicate.Register("auth", &billingConfig{})
	if err := duplicate.Init(); err == nil {
		t.Error("Init succeeded with a duplicate namespace")
	}

	empty := cfggo.NewGroup()
	if err := empty.Init(); err == nil {
		t.Error("Init succeeded with no registered members")
	}

	dotted := cfggo.NewGroup()
	dotted.Register("a.b", &authConfig{})
	if err := dotted.Init(); err == nil {
		t.Error("Init succeeded with a dotted namespace")
	}
}

func TestGroupInitTwice(t *testing.T) {
	withArgs(t)

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := group.Init(); err == nil {
		t.Error("second Group.Init succeeded, want an error")
	}
}

func TestGroupDiagnoseAndString(t *testing.T) {
	withArgs(t)

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	group.Register("billing", &billingConfig{})
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	diagnostics := group.Diagnose()
	if len(diagnostics) != 2 {
		t.Fatalf("Diagnose returned %d members, want 2", len(diagnostics))
	}
	if _, ok := diagnostics["auth"]; !ok {
		t.Error("Diagnose is missing the auth namespace")
	}
	dump := group.String()
	for _, want := range []string{"=== auth ===", "=== billing ==="} {
		if !strings.Contains(dump, want) {
			t.Errorf("String() = %q, want it to contain %q", dump, want)
		}
	}
	if report := group.Report(); !strings.Contains(report, "auth") {
		t.Errorf("Report() = %q, want it to mention auth", report)
	}
	if got := group.Namespaces(); len(got) != 2 || got[0] != "auth" || got[1] != "billing" {
		t.Errorf("Namespaces() = %v, want [auth billing]", got)
	}
}

func TestGroupMemberEnvPrefixOptionWins(t *testing.T) {
	withArgs(t)
	t.Setenv("CUSTOM_PORT", "5150")

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithEnvPrefix("MYAPP_"))
	group.Register("auth", auth, cfggo.WithEnvPrefix("CUSTOM_"))
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 5150 {
		t.Errorf("auth port = %d, want 5150 from the member's own env prefix", got)
	}
}

func TestGroupWithoutFlags(t *testing.T) {
	withArgs(t, "--auth.port=1")

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 8080 {
		t.Errorf("auth port = %d, want the default 8080 with flags disabled", got)
	}
}

func TestGroupDefaultFileConfigMissingIsNotFatal(t *testing.T) {
	withArgs(t)

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithDefaultFileConfig(filepath.Join(t.TempDir(), "absent.json")))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 8080 {
		t.Errorf("auth port = %d, want the default 8080", got)
	}
}

func TestGroupMissingRequiredFileIsFatal(t *testing.T) {
	withArgs(t)

	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(filepath.Join(t.TempDir(), "absent.json")))
	group.Register("auth", &authConfig{})
	if err := group.Init(); err == nil {
		t.Error("Group.Init succeeded with a missing required configuration file")
	}
}

// Standalone use of a type that is also used inside a group must be unaffected,
// including its unprefixed flag and env names.
func TestGroupDoesNotAffectStandaloneUse(t *testing.T) {
	withArgs(t, "--port=9999")

	standalone := &authConfig{}
	if err := cfggo.Init(standalone); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := standalone.Port(); got != 9999 {
		t.Errorf("standalone port = %d, want 9999 from the unprefixed flag", got)
	}
}

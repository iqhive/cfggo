package cfggo_test

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/cfglogger"
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

type errorGroupHandler struct {
	data      json.RawMessage
	loadErr   error
	isDefault bool
}

func (h *errorGroupHandler) IsDefault() bool { return h.isDefault }

func (h *errorGroupHandler) LoadConfig() (json.RawMessage, error) {
	if h.loadErr != nil {
		return nil, h.loadErr
	}
	return h.data, nil
}

func (h *errorGroupHandler) SaveConfig(json.RawMessage) error { return nil }

type blockingGroupSaveHandler struct {
	data        json.RawMessage
	saveStarted chan struct{}
	releaseSave chan struct{}
	saves       int
}

func (h *blockingGroupSaveHandler) IsDefault() bool { return false }

func (h *blockingGroupSaveHandler) LoadConfig() (json.RawMessage, error) {
	return h.data, nil
}

func (h *blockingGroupSaveHandler) SaveConfig(data json.RawMessage) error {
	select {
	case h.saveStarted <- struct{}{}:
	default:
	}
	<-h.releaseSave
	h.data = data
	h.saves++
	return nil
}

func TestGroupSaveIfChangedPreservesConcurrentSet(t *testing.T) {
	withArgs(t)

	handler := &blockingGroupSaveHandler{
		data:        json.RawMessage(`{}`),
		saveStarted: make(chan struct{}, 2),
		releaseSave: make(chan struct{}),
	}
	auth := &authConfig{}
	group := cfggo.NewGroup(
		cfggo.GroupWithConfigHandler(handler),
		cfggo.GroupWithoutFlags(),
	)
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := auth.Set("port", 4444); err != nil {
		t.Fatalf("Set first: %v", err)
	}

	saveDone := make(chan error, 1)
	go func() { saveDone <- group.SaveIfChanged() }()
	<-handler.saveStarted

	if err := auth.Set("port", 5555); err != nil {
		t.Fatalf("Set second: %v", err)
	}
	close(handler.releaseSave)
	if err := <-saveDone; err != nil {
		t.Fatalf("first Group.SaveIfChanged: %v", err)
	}

	if err := group.SaveIfChanged(); err != nil {
		t.Fatalf("second Group.SaveIfChanged: %v", err)
	}
	if handler.saves != 2 {
		t.Fatalf("SaveConfig calls = %d, want 2", handler.saves)
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

func TestGroupWithStandardFlags(t *testing.T) {
	withArgs(t, "--auth.port=7171")

	original := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("grouptest", flag.ContinueOnError)
	defer func() { flag.CommandLine = original }()

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithStandardFlags())
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if group.FlagSet() != flag.CommandLine {
		t.Error("GroupWithStandardFlags should register on flag.CommandLine")
	}
	if flag.CommandLine.Lookup("auth.port") == nil {
		t.Error("auth.port not registered on flag.CommandLine")
	}
	if err := flag.CommandLine.Parse([]string{"--auth.port=7171"}); err != nil {
		t.Fatalf("flag.CommandLine.Parse: %v", err)
	}
	if got := auth.Port(); got != 7171 {
		t.Errorf("auth port = %d, want 7171 from flag.CommandLine", got)
	}
}

func TestGroupWithLogger(t *testing.T) {
	withArgs(t)

	var logs strings.Builder
	logger := cfglogger.NewDefaultLoggerWithWriter(&logs)

	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithLogger(logger))
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	// The logger must be wired for group-level messages; exercise a path that
	// logs (an unknown flag) to confirm the logger is used without panicking.
	if err := group.Reload(); err != nil {
		t.Fatalf("Group.Reload: %v", err)
	}
}

func TestGroupLoadDocumentErrors(t *testing.T) {
	withArgs(t)

	t.Run("required_source_load_error", func(t *testing.T) {
		handler := &errorGroupHandler{loadErr: errors.New("boom")}
		group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
		group.Register("auth", &authConfig{})
		if err := group.Init(); err == nil {
			t.Fatal("Group.Init succeeded, want a load error")
		}
	})

	t.Run("default_source_load_error_is_non_fatal", func(t *testing.T) {
		handler := &errorGroupHandler{loadErr: errors.New("boom"), isDefault: true}
		auth := &authConfig{}
		group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
		group.Register("auth", auth)
		if err := group.Init(); err != nil {
			t.Fatalf("Group.Init: %v", err)
		}
		if got := auth.Port(); got != 8080 {
			t.Errorf("auth port = %d, want the default 8080", got)
		}
	})

	t.Run("malformed_json_is_fatal", func(t *testing.T) {
		handler := &errorGroupHandler{data: json.RawMessage(`{not-json}`)}
		group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
		group.Register("auth", &authConfig{})
		if err := group.Init(); err == nil {
			t.Fatal("Group.Init succeeded, want a parse error")
		}
	})

	t.Run("malformed_json_default_is_non_fatal", func(t *testing.T) {
		handler := &errorGroupHandler{data: json.RawMessage(`{not-json}`), isDefault: true}
		auth := &authConfig{}
		group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
		group.Register("auth", auth)
		if err := group.Init(); err != nil {
			t.Fatalf("Group.Init: %v", err)
		}
		if got := auth.Port(); got != 8080 {
			t.Errorf("auth port = %d, want the default 8080", got)
		}
	})
}

func TestGroupRecheckMemberValidation(t *testing.T) {
	withArgs(t, "--billing.port=70000")

	group := cfggo.NewGroup()
	group.Register("billing", &billingConfig{}, cfggo.WithValidation("port", validcfg.Range(1, 65535)))
	if err := group.Init(); err == nil {
		t.Fatal("Group.Init succeeded, want a recheck validation error for billing.port")
	}
}

func TestGroupSaveRootMember(t *testing.T) {
	path := writeJSON(t, `{"log_level":"info","auth":{"port":1111}}`)
	withArgs(t)

	root := &rootConfig{}
	auth := &authConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("", root)
	group.Register("auth", auth)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := root.Set("log_level", "debug"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := group.Save(); err != nil {
		t.Fatalf("Group.Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var document map[string]interface{}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal saved config: %v (%s)", err, data)
	}
	if got := document["log_level"]; got != "debug" {
		t.Errorf("saved root log_level = %v, want debug", got)
	}
	authSection, ok := document["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("saved auth section = %#v, want an object", document["auth"])
	}
	if got := authSection["port"]; got != float64(1111) {
		t.Errorf("saved auth.port = %v, want 1111", got)
	}
}

func TestGroupSaveWithoutHandler(t *testing.T) {
	withArgs(t)

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if err := group.Save(); err != nil {
		t.Fatalf("Group.Save without handler = %v, want nil", err)
	}
}

func TestGroupSuggestFlagRootMember(t *testing.T) {
	withArgs(t, "--log_leve=debug")

	group := cfggo.NewGroup()
	group.Register("", &rootConfig{})
	err := group.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want an unknown-flag error")
	}
	if !strings.Contains(err.Error(), "log_level") {
		t.Errorf("error = %v, want a suggestion of log_level", err)
	}
}

func TestGroupSuggestFlagUnknownNamespace(t *testing.T) {
	withArgs(t, "--nope.port=1")

	group := cfggo.NewGroup()
	group.Register("auth", &authConfig{})
	err := group.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want an unknown-flag error")
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Errorf("error = %v, want no suggestion for an unknown namespace", err)
	}
}

func TestGroupRecheckMemberLenient(t *testing.T) {
	path := writeJSON(t, `{"billing":{"port":70000}}`)
	withArgs(t)

	billing := &billingConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithFileConfig(path))
	group.Register("billing", billing,
		cfggo.WithValidation("port", validcfg.Range(1, 65535)),
		cfggo.WithLenientLoad(),
	)
	if err := group.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
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

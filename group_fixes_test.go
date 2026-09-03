package cfggo_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/cfglogger"
)

func setGroupTestArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = append([]string{"grouptest"}, args...)
}

type rootWithAuthGroup struct {
	cfggo.Structure
	Auth struct {
		Port func() int `cfggo:"port"`
	} `cfggo:"auth"`
}

type authMember struct {
	cfggo.Structure
	Port func() int `cfggo:"port"`
}

func TestGroupRejectsRootKeyWhosePrefixIsANamespace(t *testing.T) {
	setGroupTestArgs(t)
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("", &rootWithAuthGroup{}, cfggo.WithoutEnv())
	g.Register("auth", &authMember{}, cfggo.WithoutEnv())
	err := g.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded: root key auth.port would share flags, env vars and the file section with namespace auth")
	}
	if !strings.Contains(err.Error(), `"auth.port"`) || !strings.Contains(err.Error(), `namespace "auth"`) {
		t.Fatalf("Group.Init error = %v, want the colliding key and namespace named", err)
	}
}

type tokenMember struct {
	cfggo.Structure
	Token func() string `cfggo:"token"`
}

func TestGroupRequiredFlagOnlyKeyWithOwnFlagSet(t *testing.T) {
	setGroupTestArgs(t, "--svc.token=abc")
	m := &tokenMember{}
	g := cfggo.NewGroup()
	g.Register("svc", m, cfggo.WithoutEnv(), cfggo.WithValidation("token", cfggo.Required()))
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init = %v, want success: the required value arrives with the group's own parse", err)
	}
	if got := m.Token(); got != "abc" {
		t.Fatalf("Token() = %q", got)
	}
}

func TestGroupValidateAfterExternalFlagSetParse(t *testing.T) {
	setGroupTestArgs(t, "--svc.token=abc")
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	m := &tokenMember{}
	g := cfggo.NewGroup(cfggo.GroupWithFlagSet(fs))
	g.Register("svc", m, cfggo.WithoutEnv(), cfggo.WithValidation("token", cfggo.Required()))
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init = %v, want validation deferred for the pending --svc.token flag", err)
	}
	if err := g.Validate(); !errors.Is(err, cfggo.ErrValidation) {
		t.Fatalf("Group.Validate before Parse = %v, want the empty token reported", err)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("Group.Validate after Parse: %v", err)
	}
	if got := m.Token(); got != "abc" {
		t.Fatalf("Token() = %q", got)
	}
}

type secretMember struct {
	cfggo.Structure
	Token func() string `cfggo:"token" secret:"true"`
}

func TestGroupParseErrorRedactsSecretFlagValues(t *testing.T) {
	const raw = "group-RAW-SECRET-VALUE"
	setGroupTestArgs(t, "--auth.token="+raw)
	m := &secretMember{}
	g := cfggo.NewGroup()
	g.Register("auth", m, cfggo.WithoutEnv(), cfggo.WithValidation("token", cfggo.MinLength(64)))
	err := g.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded, want the short secret rejected")
	}
	if strings.Contains(err.Error(), raw) {
		t.Fatalf("Group.Init error leaks the raw secret: %v", err)
	}
	if !strings.Contains(err.Error(), `"****"`) {
		t.Fatalf("Group.Init error = %v, want the masked flag message", err)
	}
}

type rootUnderscoreKey struct {
	cfggo.Structure
	AuthPort func() int `cfggo:"auth_port"`
}

func TestGroupRejectsEnvVarCollisionAcrossMembers(t *testing.T) {
	setGroupTestArgs(t)
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("", &rootUnderscoreKey{})
	g.Register("auth", &authMember{})
	err := g.Init()
	if err == nil {
		t.Fatal("Group.Init succeeded although root key auth_port and auth.port both read AUTH_PORT")
	}
	if !strings.Contains(err.Error(), "AUTH_PORT") {
		t.Fatalf("Group.Init error = %v, want the shared variable named", err)
	}
}

type underscoreKeyMember struct {
	cfggo.Structure
	XY func() int `cfggo:"x_y"`
}

type yMember struct {
	cfggo.Structure
	Y func() int `cfggo:"y"`
}

func TestGroupRejectsEnvVarCollisionBetweenNamespaces(t *testing.T) {
	setGroupTestArgs(t)
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags(), cfggo.GroupWithEnvPrefix("APP_"))
	g.Register("auth", &underscoreKeyMember{})
	g.Register("auth_x", &yMember{})
	err := g.Init()
	if err == nil || !strings.Contains(err.Error(), "APP_AUTH_X_Y") {
		t.Fatalf("Group.Init error = %v, want APP_AUTH_X_Y reported as shared", err)
	}
}

func TestGroupEnvCollisionIgnoresMembersWithoutEnv(t *testing.T) {
	setGroupTestArgs(t)
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("", &rootUnderscoreKey{}, cfggo.WithoutEnv())
	g.Register("auth", &authMember{})
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init = %v, want success: the root member does not read the environment", err)
	}
}

type mutableGroupHandler struct {
	mu    sync.Mutex
	data  json.RawMessage
	saved json.RawMessage
}

func (h *mutableGroupHandler) IsDefault() bool { return false }
func (h *mutableGroupHandler) LoadConfig() (json.RawMessage, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.data, nil
}
func (h *mutableGroupHandler) SaveConfig(data json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.saved = append(json.RawMessage(nil), data...)
	return nil
}
func (h *mutableGroupHandler) set(data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = json.RawMessage(data)
}
func (h *mutableGroupHandler) lastSaved() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return string(h.saved)
}

type retryAuth struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"1"`
}

type retryBilling struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"2"`
}

func TestGroupInitFailureRevertsMembersAndCanBeRetried(t *testing.T) {
	setGroupTestArgs(t, "--auth.port=8080")
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":10},"billing":{"port":"not-a-number"}}`)}
	auth, billing := &retryAuth{}, &retryBilling{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
	g.Register("auth", auth, cfggo.WithoutEnv())
	g.Register("billing", billing, cfggo.WithoutEnv())

	if err := g.Init(); err == nil {
		t.Fatal("first Group.Init succeeded, want the billing section rejected")
	}
	if got := auth.Port(); got != 1 {
		t.Fatalf("auth.Port() after failed group init = %d, want the default 1 (member reverted)", got)
	}

	handler.set(`{"auth":{"port":10},"billing":{"port":20}}`)
	if err := g.Init(); err != nil {
		t.Fatalf("retried Group.Init: %v", err)
	}
	if got := auth.Port(); got != 8080 {
		t.Errorf("auth.Port() = %d, want 8080 from the flag parsed on retry", got)
	}
	if got := billing.Port(); got != 20 {
		t.Errorf("billing.Port() = %d, want 20 from the fixed document", got)
	}
	if err := auth.Set("port", 9); err != nil || auth.Port() != 9 {
		t.Errorf("Set after retry: %v, Port() = %d", err, auth.Port())
	}
}

func TestGroupMemberSavePersistsCombinedDocument(t *testing.T) {
	setGroupTestArgs(t)
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":10},"billing":{"port":20}}`)}
	auth, billing := &retryAuth{}, &retryBilling{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags())
	g.Register("auth", auth, cfggo.WithoutEnv())
	g.Register("billing", billing, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}

	if err := auth.Set("port", 11); err != nil {
		t.Fatal(err)
	}
	if err := auth.Save(); err != nil {
		t.Fatalf("member Save: %v", err)
	}
	var doc map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(handler.lastSaved()), &doc); err != nil {
		t.Fatalf("member Save wrote nothing usable: %v (%q)", err, handler.lastSaved())
	}
	if doc["auth"]["port"] != float64(11) || doc["billing"]["port"] != float64(20) {
		t.Fatalf("member Save wrote %s, want the combined document with the new auth port", handler.lastSaved())
	}

	// SaveIfChanged on a member follows the same path and clears the flag
	if err := billing.Set("port", 21); err != nil {
		t.Fatal(err)
	}
	if err := billing.SaveIfChanged(); err != nil {
		t.Fatalf("member SaveIfChanged: %v", err)
	}
	if !strings.Contains(handler.lastSaved(), `"port":21`) {
		t.Fatalf("member SaveIfChanged wrote %s", handler.lastSaved())
	}
	if err := g.SaveIfChanged(); err != nil {
		t.Fatal(err)
	}
}

func TestGroupSaveOmitsEnvOverrides(t *testing.T) {
	setGroupTestArgs(t)
	t.Setenv("APP_AUTH_PORT", "555")
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":10}}`)}
	auth := &retryAuth{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags(), cfggo.GroupWithEnvPrefix("APP_"))
	g.Register("auth", auth)
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 555 {
		t.Fatalf("Port() = %d, want the env override", got)
	}
	if err := g.Save(); err != nil {
		t.Fatalf("Group.Save: %v", err)
	}
	if !strings.Contains(handler.lastSaved(), `"port":10`) || strings.Contains(handler.lastSaved(), "555") {
		t.Fatalf("Group.Save wrote %s, want the document value, not the env override", handler.lastSaved())
	}
}

// 4. Group.Reload may be called from an OnChange callback fired by Group.Reload
func TestGroupReloadIsReentrantFromCallbacks(t *testing.T) {
	setGroupTestArgs(t)
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":1}}`)}
	auth := &retryAuth{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags())
	g.Register("auth", auth, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	nested := make(chan error, 1)
	var cancel func()
	cancel = auth.OnChange(func([]cfggo.Change) {
		cancel()
		handler.set(`{"auth":{"port":3}}`)
		nested <- g.Reload() // synchronous re-entry from inside the outer reload's callback
	})
	handler.set(`{"auth":{"port":2}}`)
	done := make(chan error, 1)
	go func() { done <- g.Reload() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("outer Group.Reload: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Group.Reload deadlocked when re-entered from an OnChange callback")
	}
	if err := <-nested; err != nil {
		t.Fatalf("nested Group.Reload: %v", err)
	}
	if got := auth.Port(); got != 3 {
		t.Fatalf("Port() = %d, want 3 from the nested reload", got)
	}
}

// 6. unclaimed sections are reported with a namespace suggestion
func TestGroupReportsUnclaimedSectionWithSuggestion(t *testing.T) {
	setGroupTestArgs(t)
	var logs bytes.Buffer
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":1},"billng":{"port":2},"debug":true}`)}
	auth, billing := &retryAuth{}, &retryBilling{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags(),
		cfggo.GroupWithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs)))
	g.Register("auth", auth, cfggo.WithoutEnv())
	g.Register("billing", billing, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	out := logs.String()
	if !strings.Contains(out, "billng") || !strings.Contains(out, `did you mean \"billing\"?`) {
		t.Errorf("expected a warning naming section billng with a billing suggestion, got:\n%s", out)
	}
	if !strings.Contains(out, "section=debug") {
		t.Errorf("expected the scalar top-level key debug to be reported as unclaimed, got:\n%s", out)
	}
	// The suggestion is advisory only: billing keeps its default
	if got := billing.Port(); got != 2 {
		t.Errorf("billing.Port() = %d, want the default 2; the misspelt section must not be applied", got)
	}
}

func TestGroupStrictKeysRejectsUnclaimedSection(t *testing.T) {
	setGroupTestArgs(t)
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":1},"billng":{"port":2}}`)}
	for name, opts := range map[string][]cfggo.GroupOption{
		"GroupWithStrictKeys": {cfggo.GroupWithStrictKeys()},
		"all members strict":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			auth, billing := &retryAuth{}, &retryBilling{}
			memberOpts := []cfggo.Option{cfggo.WithoutEnv()}
			if opts == nil {
				memberOpts = append(memberOpts, cfggo.WithStrictKeys())
			}
			g := cfggo.NewGroup(append(opts, cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags())...)
			g.Register("auth", auth, memberOpts...)
			g.Register("billing", billing, memberOpts...)
			err := g.Init()
			if !errors.Is(err, cfggo.ErrUnknownKey) {
				t.Fatalf("Group.Init = %v, want ErrUnknownKey for the unclaimed section", err)
			}
			if !strings.Contains(err.Error(), `"billng"`) || !strings.Contains(err.Error(), `did you mean "billing"?`) {
				t.Fatalf("Group.Init error = %v, want the section named with a suggestion", err)
			}
			// a retry with the fixed document must work (members were reverted)
			handler.set(`{"auth":{"port":1},"billing":{"port":2}}`)
			if err := g.Init(); err != nil {
				t.Fatalf("Group.Init after fixing the section: %v", err)
			}
			handler.set(`{"auth":{"port":1},"billng":{"port":2}}`)
		})
	}
}

func TestGroupRootMemberGroupsAreNotUnclaimed(t *testing.T) {
	setGroupTestArgs(t)
	var logs bytes.Buffer
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":1},"db":{"host":"h"}}`)}
	root := &rootWithDBGroup{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags(), cfggo.GroupWithStrictKeys(),
		cfggo.GroupWithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs)))
	g.Register("", root, cfggo.WithoutEnv())
	g.Register("auth", &retryAuth{}, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v (a root member's nested group is a claimed section)", err)
	}
	if got := root.DB.Host(); got != "h" {
		t.Fatalf("root DB.Host() = %q", got)
	}
	if strings.Contains(logs.String(), "unclaimed") {
		t.Fatalf("unexpected unclaimed-section warning:\n%s", logs.String())
	}
}

type rootWithDBGroup struct {
	cfggo.Structure
	DB struct {
		Host func() string `cfggo:"host"`
	} `cfggo:"db"`
}

// 7. plain fields are not configuration keys for collision purposes
type rootWithPlainField struct {
	cfggo.Structure
	Extra int        // ordinary state, not a config value
	Port  func() int `cfggo:"port"`
}

type rootWithExtraAccessor struct {
	cfggo.Structure
	Extra func() int `cfggo:"Extra"`
}

func TestGroupIgnoresPlainFieldsInCollisionChecks(t *testing.T) {
	setGroupTestArgs(t)
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("", &rootWithPlainField{}, cfggo.WithoutEnv())
	g.Register("", &rootWithExtraAccessor{}, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init = %v, want success: a plain Extra field is not a key", err)
	}
}

// 5. a member embedding *cfggo.Structure by pointer
type pointerEmbedMember struct {
	*cfggo.Structure
	Port func() int `cfggo:"port" default:"9"`
}

func TestGroupRegisterAllocatesNilEmbeddedStructure(t *testing.T) {
	setGroupTestArgs(t)
	m := &pointerEmbedMember{}
	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("svc", m, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := m.Port(); got != 9 {
		t.Fatalf("Port() = %d", got)
	}
}

func TestGroupDroppedFlagLogsDoNotEchoValues(t *testing.T) {
	setGroupTestArgs(t, "--other-token=SECRET-VALUE", "--auth.port=7", "stray-SECRET")
	var logs bytes.Buffer
	logger := cfglogger.NewDefaultLoggerWithWriter(&logs)
	logger.SetLevel(-8)
	auth := &retryAuth{}
	g := cfggo.NewGroup(cfggo.GroupWithIgnoreUnknownFlags(), cfggo.GroupWithLogger(logger))
	g.Register("auth", auth, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init: %v", err)
	}
	if got := auth.Port(); got != 7 {
		t.Fatalf("Port() = %d", got)
	}
	out := logs.String()
	if strings.Contains(out, "SECRET") {
		t.Fatalf("group logs echo a dropped value:\n%s", out)
	}
	if !strings.Contains(out, "flag=other-token") {
		t.Fatalf("group logs should name the dropped flag:\n%s", out)
	}
}

func TestGroupIgnoredSectionsAreNeitherReportedNorDelivered(t *testing.T) {
	setGroupTestArgs(t)
	var logs bytes.Buffer
	handler := &mutableGroupHandler{data: json.RawMessage(`{"auth":{"port":1},"other-binary":{"port":2},"db":{"host":"h"}}`)}
	root := &rootWithDBGroup{}
	auth := &retryAuth{}
	g := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler), cfggo.GroupWithoutFlags(), cfggo.GroupWithStrictKeys(),
		cfggo.GroupWithIgnoredSections("other-binary"),
		cfggo.GroupWithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs)))
	g.Register("", root, cfggo.WithoutEnv())
	g.Register("auth", auth, cfggo.WithoutEnv())
	if err := g.Init(); err != nil {
		t.Fatalf("Group.Init = %v, want success: the foreign section is ignored", err)
	}
	if strings.Contains(logs.String(), "other-binary") {
		t.Fatalf("ignored section was still reported:\n%s", logs.String())
	}
	if _, ok := root.Get("other-binary.port"); ok {
		t.Fatal("ignored section was delivered to the root member")
	}
	if root.DB.Host() != "h" || auth.Port() != 1 {
		t.Fatalf("values: db.host=%q auth.port=%d", root.DB.Host(), auth.Port())
	}
	handler.set(`{"auth":{"port":5},"other-binary":{"port":6}}`)
	if err := g.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if strings.Contains(logs.String(), "other-binary") {
		t.Fatalf("ignored section reported on reload:\n%s", logs.String())
	}
}

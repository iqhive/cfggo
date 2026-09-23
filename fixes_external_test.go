package cfggo_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/validcfg"
)

// withArgs is shared with group_test.go

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type flagFixConfig struct {
	cfggo.Structure
	Port  func() int  `cfggo:"port" default:"8080"`
	Debug func() bool `cfggo:"debug" default:"false"`
}

// A key that is not a legal flag name (here one loaded from the config file)
// must not make Init panic inside the flag package.
func TestIllegalFlagNameFromConfigFileDoesNotPanic(t *testing.T) {
	withArgs(t)
	file := writeTemp(t, "c.json", `{"port": 1, "a=b": 2, "-dash": 3}`)
	cfg := &flagFixConfig{}
	if err := cfg.Init(cfg, cfggo.WithFileConfig(file), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 1 {
		t.Fatalf("Port() = %d, want 1", got)
	}
}

type secretBoolConfig struct {
	cfggo.Structure
	Hidden func() bool `cfggo:"hidden" default:"false" secret:"true"`
}

// A rejected value for a secret-tagged boolean flag must be redacted like any
// other secret flag value.
func TestSecretBoolFlagValueIsRedacted(t *testing.T) {
	withArgs(t, "--hidden=notabool-s3cr3t")
	cfg := &secretBoolConfig{}
	err := cfg.Init(cfg, cfggo.WithoutEnv())
	if err == nil {
		t.Fatal("Init should have failed on the bad boolean")
	}
	if strings.Contains(err.Error(), "s3cr3t") {
		t.Fatalf("secret flag value leaked into the error: %v", err)
	}
}

// A validation failure raised while a flag is parsed must still match the
// ErrValidation sentinel, although the flag package flattens the error text.
func TestFlagValidationFailureKeepsSentinel(t *testing.T) {
	withArgs(t, "--port=70000")
	cfg := &flagFixConfig{}
	err := cfg.Init(cfg, cfggo.WithoutEnv(), cfggo.WithValidation("port", cfggo.Range(1, 65535)))
	if err == nil {
		t.Fatal("Init should have failed validation")
	}
	if !errors.Is(err, cfggo.ErrValidation) {
		t.Fatalf("errors.Is(err, ErrValidation) = false: %v", err)
	}
	var ve validcfg.ValidationError
	if !errors.As(err, &ve) || ve.Key != "port" {
		t.Fatalf("errors.As(err, &ValidationError) failed or wrong key: %v", err)
	}
}

// An unknown flag that follows a "--bool value" pair must be reported as an
// unknown key with a suggestion, like any other unknown flag.
func TestUnknownFlagAfterBoolValuePairIsClassified(t *testing.T) {
	withArgs(t, "--debug", "true", "--prot", "1")
	cfg := &flagFixConfig{}
	err := cfg.Init(cfg, cfggo.WithoutEnv())
	if err == nil {
		t.Fatal("Init should have failed on the unknown flag")
	}
	if !errors.Is(err, cfggo.ErrUnknownKey) {
		t.Fatalf("errors.Is(err, ErrUnknownKey) = false: %v", err)
	}
	if !strings.Contains(err.Error(), "did you mean -port") {
		t.Fatalf("no suggestion in error: %v", err)
	}
}

type Toggle bool

type namedBoolConfig struct {
	cfggo.Structure
	Verbose func() Toggle `cfggo:"verbose" default:"false"`
	Name    func() string `cfggo:"name" default:"x"`
}

// A named bool type is a boolean flag: a bare --verbose must not swallow the
// following flag as its value.
func TestNamedBoolTypeIsBooleanFlag(t *testing.T) {
	withArgs(t, "--verbose", "--name=set")
	cfg := &namedBoolConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !bool(cfg.Verbose()) {
		t.Fatal("Verbose() = false, want true")
	}
	if got := cfg.Name(); got != "set" {
		t.Fatalf("Name() = %q, want set", got)
	}
}

type anyConfig struct {
	cfggo.Structure
	Any func() interface{} `cfggo:"any"`
}

// A func() interface{} field accepts values of any type, whatever type its
// current value happens to have.
func TestInterfaceFieldIsNotPinnedToFirstValueType(t *testing.T) {
	cfg := &anyConfig{Any: cfggo.DefaultValue[interface{}]("text")}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("any", 42); err != nil {
		t.Fatalf("Set(any, 42): %v", err)
	}
	if got := cfg.Any(); got != 42 {
		t.Fatalf("Any() = %#v, want int 42", got)
	}
	if err := cfg.Set("any", []interface{}{1.0, 2.0}); err != nil {
		t.Fatalf("Set(any, slice): %v", err)
	}
	if got, ok := cfg.Any().([]interface{}); !ok || len(got) != 2 {
		t.Fatalf("Any() = %#v, want a 2-element slice", cfg.Any())
	}
	if err := cfg.Set("any", "back-to-text"); err != nil {
		t.Fatalf("Set(any, text): %v", err)
	}
	if got := cfg.Any(); got != "back-to-text" {
		t.Fatalf("Any() = %#v, want back-to-text", got)
	}
}

type defaultFileConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"8080"`
}

// WithDefaultFileConfig tolerates an absent file, not a present but malformed
// one: that is a hard error unless WithLenientLoad is given.
func TestDefaultFileConfigRejectsMalformedContent(t *testing.T) {
	bad := writeTemp(t, "bad.json", `{"port": `)
	cfg := &defaultFileConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithDefaultFileConfig(bad)); err == nil {
		t.Fatal("Init accepted a malformed optional configuration file")
	}

	typed := writeTemp(t, "typed.json", `{"port": "not-a-port"}`)
	cfg = &defaultFileConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithDefaultFileConfig(typed)); err == nil {
		t.Fatal("Init accepted an optional configuration file with an uncoercible value")
	}

	cfg = &defaultFileConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithDefaultFileConfig(bad), cfggo.WithLenientLoad()); err != nil {
		t.Fatalf("Init with WithLenientLoad: %v", err)
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want the default 8080", got)
	}

	cfg = &defaultFileConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithDefaultFileConfig(filepath.Join(t.TempDir(), "absent.json"))); err != nil {
		t.Fatalf("Init with an absent optional file: %v", err)
	}
}

type dupKeyConfig struct {
	cfggo.Structure
	A func() int    `cfggo:"same"`
	B func() string `cfggo:"same"`
}

func TestDuplicateKeysAreRejected(t *testing.T) {
	cfg := &dupKeyConfig{}
	err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv())
	if err == nil {
		t.Fatal("Init accepted two accessors with the same key")
	}
	if !strings.Contains(err.Error(), `"same"`) || !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "B") {
		t.Fatalf("error does not name the key and both fields: %v", err)
	}
}

type omitemptyConfig struct {
	cfggo.Structure
	Level func() string `json:",omitempty" default:"info"`
}

// A tag that carries only options names the field after itself, as
// encoding/json does, rather than dropping it.
func TestEmptyNameTagFallsBackToFieldName(t *testing.T) {
	cfg := &omitemptyConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Level(); got != "info" {
		t.Fatalf("Level() = %q, want info", got)
	}
	if _, ok := cfg.Get("Level"); !ok {
		t.Fatal("key Level not registered")
	}
}

type plainPointerConfig struct {
	cfggo.Structure
	Port   func() int `cfggo:"port" default:"1"`
	Client *http.Client
	Group  *plainPointerGroup
}

type plainPointerGroup struct {
	Timeout func() int `cfggo:"timeout" default:"5"`
}

// Only pointer fields that actually hold configuration are allocated by Init.
func TestInitAllocatesOnlyConfigurationGroups(t *testing.T) {
	cfg := &plainPointerConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if cfg.Client != nil {
		t.Fatal("Init allocated a *http.Client that holds no configuration")
	}
	if cfg.Group == nil || cfg.Group.Timeout() != 5 {
		t.Fatal("Init did not allocate and wire the configuration group")
	}
}

type hiddenGroup struct {
	Timeout func() int `cfggo:"timeout" default:"5"`
}

type nilUnexportedEmbeddedConfig struct {
	cfggo.Structure
	*hiddenGroup
}

// A nil unexported embedded pointer group is reported, not dereferenced.
func TestNilUnexportedEmbeddedPointerGroupIsAnError(t *testing.T) {
	cfg := &nilUnexportedEmbeddedConfig{}
	err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv())
	if err == nil {
		t.Fatal("Init succeeded with a nil unexported embedded pointer group")
	}
	if !strings.Contains(err.Error(), "hiddenGroup") {
		t.Fatalf("error does not name the field: %v", err)
	}

	// Allocated up front it works
	cfg = &nilUnexportedEmbeddedConfig{hiddenGroup: &hiddenGroup{}}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init with the group allocated: %v", err)
	}
	if got := cfg.Timeout(); got != 5 {
		t.Fatalf("Timeout() = %d, want 5", got)
	}
}

type credentials struct {
	User     func() string `cfggo:"user" default:"admin"`
	Password func() string `cfggo:"password" default:"hunter2"`
}

type secretGroupConfig struct {
	cfggo.Structure
	DB credentials `cfggo:"db" secret:"true"`
}

// A group tagged secret:"true" masks every value beneath it.
func TestSecretGroupMasksAllLeaves(t *testing.T) {
	cfg := &secretGroupConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for name, dump := range map[string]string{"String": cfg.String(), "Explain": cfg.Explain(), "Report": cfg.Report()} {
		if strings.Contains(dump, "hunter2") || strings.Contains(dump, "admin") {
			t.Fatalf("%s() printed a value of a secret group:\n%s", name, dump)
		}
	}
}

type innerOptions struct {
	Tags []string
}

type outerOptions struct {
	Inner *innerOptions
}

type ptrInStructConfig struct {
	cfggo.Structure
	Opts func() outerOptions `cfggo:"opts"`
}

// A pointer inside a struct-typed value must be cloned on read: mutating the
// value a caller received must not change live configuration.
func TestStructValuePointerFieldsAreCloned(t *testing.T) {
	cfg := &ptrInStructConfig{Opts: cfggo.DefaultValue(outerOptions{Inner: &innerOptions{Tags: []string{"a"}}})}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	got := cfg.Opts()
	got.Inner.Tags[0] = "mutated"
	got.Inner.Tags = append(got.Inner.Tags, "extra")
	if again := cfg.Opts(); again.Inner.Tags[0] != "a" || len(again.Inner.Tags) != 1 {
		t.Fatalf("live configuration was mutated through a returned pointer: %#v", again.Inner.Tags)
	}
}

// Init with a pointer to a non-struct is an error, not a panic.
func TestInitPointerToNonStructIsAnError(t *testing.T) {
	var s cfggo.Structure
	n := 42
	if err := s.Init(&n, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err == nil {
		t.Fatal("Init accepted a pointer to an int")
	}
}

type orderConfig struct {
	cfggo.Structure
	Alpha func() string `cfggo:"alpha" default:""`
	Beta  func() string `cfggo:"beta" default:""`
	Gamma func() string `cfggo:"gamma" default:""`
}

// The aggregate validation error lists keys in a stable (sorted) order.
func TestValidationErrorsAreOrderedByKey(t *testing.T) {
	for i := 0; i < 5; i++ {
		cfg := &orderConfig{}
		err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(),
			cfggo.WithValidation("gamma", cfggo.Required()),
			cfggo.WithValidation("alpha", cfggo.Required()),
			cfggo.WithValidation("beta", cfggo.Required()))
		if err == nil {
			t.Fatal("Init should have failed validation")
		}
		var errs validcfg.ValidationErrors
		if !errors.As(err, &errs) || len(errs) != 3 {
			t.Fatalf("want 3 validation errors, got %v", err)
		}
		if errs[0].Key != "alpha" || errs[1].Key != "beta" || errs[2].Key != "gamma" {
			t.Fatalf("validation errors not in key order: %v %v %v", errs[0].Key, errs[1].Key, errs[2].Key)
		}
	}
}

type sliceConfig struct {
	cfggo.Structure
	Tags func() []string `cfggo:"tags"`
}

// A validator receives a copy of the value, so it cannot mutate live
// configuration in place.
func TestValidateKeyHandsValidatorACopy(t *testing.T) {
	cfg := &sliceConfig{Tags: cfggo.DefaultValue([]string{"a", "b"})}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg.RegisterValidator("tags", func(v interface{}) error {
		v.([]string)[0] = "mutated"
		return nil
	})
	if err := cfg.ValidateKey("tags"); err != nil {
		t.Fatalf("ValidateKey: %v", err)
	}
	if got := cfg.Tags(); got[0] != "a" {
		t.Fatalf("validator mutated live configuration: %v", got)
	}
}

type groupFixConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"8080"`
}

type valueEmbedsPointer struct {
	*cfggo.Structure
	Port func() int `cfggo:"port" default:"1"`
}

// Registration problems that used to panic during Init are reported.
func TestGroupRegisterRejectsPanicInducingInputs(t *testing.T) {
	for _, ns := range []string{"-dash", "a=b"} {
		g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
		g.Register(ns, &groupFixConfig{})
		if err := g.Init(); err == nil {
			t.Fatalf("Init accepted namespace %q", ns)
		}
	}

	g := cfggo.NewGroup(cfggo.GroupWithoutFlags())
	g.Register("", valueEmbedsPointer{})
	if err := g.Init(); err == nil {
		t.Fatal("Init accepted a struct value with a nil embedded *Structure")
	}
}

// Save and Reload before Init are errors: the members hold nothing yet, so a
// save would empty every section of the combined document.
func TestGroupSaveAndReloadBeforeInitAreErrors(t *testing.T) {
	file := writeTemp(t, "combined.json", `{"auth":{"port":1}}`)
	g := cfggo.NewGroup(cfggo.GroupWithFileConfig(file), cfggo.GroupWithoutFlags())
	g.Register("auth", &groupFixConfig{})
	if err := g.Save(); err == nil {
		t.Fatal("Save before Init succeeded")
	}
	if err := g.SaveIfChanged(); err == nil {
		t.Fatal("SaveIfChanged before Init succeeded")
	}
	if err := g.Reload(); err == nil {
		t.Fatal("Reload before Init succeeded")
	}
	data, _ := os.ReadFile(file)
	if string(data) != `{"auth":{"port":1}}` {
		t.Fatalf("combined document was modified before Init: %s", data)
	}
}

// A root member's NewFlag key is the member's, so the section it occupies in
// the combined document is claimed under GroupWithStrictKeys.
func TestGroupRootMemberNewFlagKeyClaimsItsSection(t *testing.T) {
	file := writeTemp(t, "combined.json", `{"port": 5, "region": "eu"}`)
	root := &groupFixConfig{}
	root.NewFlag("region", "us", "deployment region")
	g := cfggo.NewGroup(cfggo.GroupWithFileConfig(file), cfggo.GroupWithoutFlags(), cfggo.GroupWithStrictKeys())
	g.Register("", root)
	if err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if v, _ := root.Get("region"); v != "eu" {
		t.Fatalf("region = %v, want eu", v)
	}
}

// A validation failure raised while the group parses its shared flag set
// must keep the ErrValidation sentinel.
func TestGroupFlagValidationFailureKeepsSentinel(t *testing.T) {
	withArgs(t, "--auth.port=70000")
	g := cfggo.NewGroup()
	g.Register("auth", &groupFixConfig{}, cfggo.WithValidation("port", cfggo.Range(1, 65535)))
	err := g.Init()
	if err == nil {
		t.Fatal("Init should have failed validation")
	}
	if !errors.Is(err, cfggo.ErrValidation) {
		t.Fatalf("errors.Is(err, ErrValidation) = false: %v", err)
	}
}

type pointerValueConfig struct {
	cfggo.Structure
	Limit func() *int `cfggo:"limit"`
}

// A func() *T value is rendered as what it points to, not as an address.
func TestHumanDumpsDereferencePointers(t *testing.T) {
	limit := 4242
	cfg := &pointerValueConfig{Limit: cfggo.DefaultValue(&limit)}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for name, dump := range map[string]string{"String": cfg.String(), "Explain": cfg.Explain()} {
		if strings.Contains(dump, "0x") || !strings.Contains(dump, "4242") {
			t.Fatalf("%s() printed a pointer address instead of the value:\n%s", name, dump)
		}
	}
}

type nestedNullConfig struct {
	cfggo.Structure
	DB struct {
		Host func() string `cfggo:"host" default:"localhost"`
	} `cfggo:"db"`
}

// A JSON null in place of a whole section supplies no values for it; it is
// not an unrecognized key named after the section.
func TestNullSectionIsNotAnUnrecognizedKey(t *testing.T) {
	file := writeTemp(t, "c.json", `{"db": null}`)
	cfg := &nestedNullConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithFileConfig(file), cfggo.WithStrictKeys()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.DB.Host(); got != "localhost" {
		t.Fatalf("Host() = %q, want localhost", got)
	}
}

// A file holding only whitespace is as empty as a zero-length one.
func TestWhitespaceOnlyFileIsEmpty(t *testing.T) {
	file := writeTemp(t, "c.json", "\n  \n")
	cfg := &defaultFileConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv(), cfggo.WithFileConfig(file)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want 8080", got)
	}
}

var _ = json.Marshal

type listNode struct {
	Next  *listNode
	Value int
}

type plainRecursiveConfig struct {
	cfggo.Structure
	Port func() int `cfggo:"port" default:"1"`
	List *listNode
}

type recursiveGroup struct {
	Timeout  func() int `cfggo:"timeout" default:"1"`
	Fallback *recursiveGroup
}

type recursiveGroupConfig struct {
	cfggo.Structure
	Server recursiveGroup `cfggo:"server"`
}

// A recursive value type that holds no configuration is simply not a group;
// a recursive group that does hold configuration is still rejected.
func TestRecursivePlainTypesAreToleratedRecursiveGroupsRejected(t *testing.T) {
	cfg := &plainRecursiveConfig{}
	if err := cfg.Init(cfg, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err != nil {
		t.Fatalf("Init with a plain recursive type: %v", err)
	}
	if cfg.List != nil {
		t.Fatal("Init allocated the plain recursive type")
	}
	if got := cfg.Port(); got != 1 {
		t.Fatalf("Port() = %d, want 1", got)
	}

	rc := &recursiveGroupConfig{}
	if err := rc.Init(rc, cfggo.WithoutFlags(), cfggo.WithoutEnv()); err == nil {
		t.Fatal("Init accepted a recursive configuration group")
	}
}

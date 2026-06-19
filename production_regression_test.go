package cfggo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iqhive/cfggo/cfglogger"
)

type mapLeafRegressionConfig struct {
	Structure
	Labels   func() map[string]string `cfggo:"labels"`
	Metadata func() map[string]int    `cfggo:"metadata"`
}

type loadFailureRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

type printfStyleRegressionLogger struct {
	out *bytes.Buffer
}

func (l *printfStyleRegressionLogger) Debug(msg string, args ...any) {
	fmt.Fprintf(l.out, msg, args...)
}

func (l *printfStyleRegressionLogger) Info(msg string, args ...any) {
	fmt.Fprintf(l.out, msg, args...)
}

func (l *printfStyleRegressionLogger) Warn(msg string, args ...any) {
	fmt.Fprintf(l.out, msg, args...)
}

func (l *printfStyleRegressionLogger) Error(msg string, args ...any) {
	fmt.Fprintf(l.out, msg, args...)
}

func TestMapLeafLoadsFromJSONObject(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{
		"labels":{"env":"prod","team":"core"},
		"metadata":{"retries":3}
	}`)}
	cfg := &mapLeafRegressionConfig{
		Labels:   DefaultValue(map[string]string{}),
		Metadata: DefaultValue(map[string]int{}),
	}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	labels := cfg.Labels()
	if labels["env"] != "prod" || labels["team"] != "core" {
		t.Fatalf("Labels() = %#v, want env/team loaded from JSON object", labels)
	}
	metadata := cfg.Metadata()
	if metadata["retries"] != 3 {
		t.Fatalf("Metadata() = %#v, want retries=3", metadata)
	}
	if _, ok := cfg.Get("labels.env"); ok {
		t.Fatal("JSON object leaf was flattened into labels.env")
	}
}

type mutableAccessorRegressionConfig struct {
	Structure
	Labels func() map[string]string `cfggo:"labels"`
	Names  func() []string          `cfggo:"names"`
	State  func() *mutableState     `cfggo:"state"`
}

type mutableState struct {
	Name string
}

func TestMutableAccessorsReturnCopies(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &mutableAccessorRegressionConfig{
		Labels: DefaultValue(map[string]string{"env": "prod"}),
		Names:  DefaultValue([]string{"api"}),
		State:  DefaultValue(&mutableState{Name: "ready"}),
	}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	labels := cfg.Labels()
	labels["env"] = "dev"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating accessor result, want prod", got)
	}

	names := cfg.Names()
	names[0] = "worker"
	if got := cfg.Names()[0]; got != "api" {
		t.Fatalf("Names()[0] = %q after mutating accessor result, want api", got)
	}

	labelsValue, ok := Value[map[string]string](&cfg.Structure, "labels")
	if !ok {
		t.Fatal("Value[map[string]string](labels) returned ok=false")
	}
	labelsValue["env"] = "qa"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating typed Value result, want prod", got)
	}

	state := cfg.State()
	state.Name = "mutated"
	if got := cfg.State().Name; got != "ready" {
		t.Fatalf("State().Name = %q after mutating accessor result, want ready", got)
	}
}

func TestSetAndGetCloneMutableValues(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &mutableAccessorRegressionConfig{
		Labels: DefaultValue(map[string]string{}),
		Names:  DefaultValue([]string{}),
		State:  DefaultValue(&mutableState{}),
	}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	labels := map[string]string{"env": "prod"}
	if err := cfg.Set("labels", labels); err != nil {
		t.Fatalf("Set labels: %v", err)
	}
	labels["env"] = "dev"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating Set input, want prod", got)
	}

	gotRaw, ok := cfg.Get("labels")
	if !ok {
		t.Fatal("Get(labels) returned ok=false")
	}
	gotLabels := gotRaw.(map[string]string)
	gotLabels["env"] = "qa"
	if got := cfg.Labels()["env"]; got != "prod" {
		t.Fatalf("Labels()[env] = %q after mutating Get result, want prod", got)
	}
}

func TestReloadChangeValuesAreCopies(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{"labels":{"env":"prod"}}`)}
	cfg := &mutableAccessorRegressionConfig{
		Labels: DefaultValue(map[string]string{}),
		Names:  DefaultValue([]string{}),
		State:  DefaultValue(&mutableState{}),
	}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	cancel := cfg.OnChange(func(changes []Change) {
		for _, ch := range changes {
			if ch.Key == "labels" {
				ch.New.(map[string]string)["env"] = "mutated"
			}
		}
	})
	defer cancel()

	handler.data = json.RawMessage(`{"labels":{"env":"stage"}}`)
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Labels()["env"]; got != "stage" {
		t.Fatalf("Labels()[env] = %q after mutating callback Change.New, want stage", got)
	}
}

func TestDefaultValueClonesMutableValuesBeforeInit(t *testing.T) {
	labels := map[string]string{"env": "prod"}
	names := []string{"api"}
	state := &mutableState{Name: "ready"}

	cfg := &mutableAccessorRegressionConfig{
		Labels: DefaultValue(labels),
		Names:  DefaultValue(names),
		State:  DefaultClone(state),
	}

	cfg.Labels()["env"] = "dev"
	cfg.Names()[0] = "worker"
	cfg.State().Name = "mutated"

	if labels["env"] != "prod" {
		t.Fatalf("original labels mutated before Init: %#v", labels)
	}
	if names[0] != "api" {
		t.Fatalf("original names mutated before Init: %#v", names)
	}
	if state.Name != "ready" {
		t.Fatalf("original pointer default mutated before Init: %#v", state)
	}
}

type reloadDefaultsRegressionConfig struct {
	Structure
	Host func() string `cfggo:"host" default:"localhost"`
}

func TestReloadRemovedFileKeyRevertsToDefault(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{"host":"fromfile"}`)}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "fromfile" {
		t.Fatalf("initial Host() = %q, want fromfile", got)
	}

	handler.data = json.RawMessage(`{}`)
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "localhost" {
		t.Fatalf("after reload Host() = %q, want default localhost", got)
	}
	if src, _ := cfg.Source("host"); src != SourceDefault {
		t.Fatalf("after reload source = %s, want default", src)
	}
}

type setOverrideRegressionConfig struct {
	Structure
	Host func() string `cfggo:"host" default:"localhost"`
}

func TestReloadPreservesProgrammaticSetOverride(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("HOST", "fromenv")

	cfg := &setOverrideRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Host(); got != "fromenv" {
		t.Fatalf("initial Host() = %q, want fromenv", got)
	}
	if err := cfg.Set("host", "runtime-override"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := cfg.Host(); got != "runtime-override" {
		t.Fatalf("after reload Host() = %q, want runtime override", got)
	}
	if src, _ := cfg.Source("host"); src != SourceSet {
		t.Fatalf("after reload source = %s, want set", src)
	}
}

type validatedReloadRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

type validatorDeadlockRegressionConfig struct {
	Structure
	Port func() int    `cfggo:"port" default:"8080"`
	Host func() string `cfggo:"host" default:"localhost"`
}

func TestReloadValidationFailureKeepsPreviousValues(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{"port":9090}`)}
	cfg := &validatedReloadRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags(), WithValidation("port", Range(1, 65535))); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("initial Port() = %d, want 9090", got)
	}

	handler.data = json.RawMessage(`{"port":70000}`)
	if err := cfg.Reload(); err == nil {
		t.Fatal("Reload: expected validation error, got nil")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("after failed reload Port() = %d, want previous valid value 9090", got)
	}
	if src, _ := cfg.Source("port"); src != SourceFile {
		t.Fatalf("after failed reload source = %s, want previous file source", src)
	}
}

func TestFlagValidatorCanReadConfigWithoutDeadlock(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test", "--port=9090"}

	cfg := &validatorDeadlockRegressionConfig{}
	done := make(chan error, 1)
	go func() {
		done <- cfg.Init(cfg, WithValidation("port", func(interface{}) error {
			_, _ = cfg.Get("host")
			return nil
		}))
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Init deadlocked while flag validator read config")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("Port() = %d, want flag value 9090", got)
	}
}

func TestValidateAllowsMutatingValidatorWithoutDeadlock(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &validatorDeadlockRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg.RegisterValidator("port", func(interface{}) error {
		return cfg.Set("host", "validator-updated")
	})

	done := make(chan error, 1)
	go func() {
		done <- cfg.Validate()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Validate deadlocked while validator mutated config")
	}
	if got := cfg.Host(); got != "validator-updated" {
		t.Fatalf("Host() = %q, want validator-updated", got)
	}
}

func TestReloadEnvConversionFailureKeepsPreviousValues(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("PORT", "9090")

	cfg := &validatedReloadRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("initial Port() = %d, want 9090", got)
	}

	if err := os.Setenv("PORT", "not-a-number"); err != nil {
		t.Fatalf("Setenv: %v", err)
	}
	if err := cfg.Reload(); err == nil {
		t.Fatal("Reload: expected env conversion error, got nil")
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("after failed env reload Port() = %d, want previous env value 9090", got)
	}
	if src, _ := cfg.Source("port"); src != SourceEnv {
		t.Fatalf("after failed env reload source = %s, want previous env source", src)
	}
}

func TestReloadStrictKeysRejectsNewUnknownKeys(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &memHandler{data: json.RawMessage(`{"port":9090}`)}
	cfg := &validatedReloadRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags(), WithStrictKeys()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	handler.data = json.RawMessage(`{"port":9091,"prot":1234}`)
	err := cfg.Reload()
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Reload error = %v, want ErrUnknownKey", err)
	}
	if got := cfg.Port(); got != 9090 {
		t.Fatalf("after strict-key reload failure Port() = %d, want previous value 9090", got)
	}
	if _, ok := cfg.Get("prot"); ok {
		t.Fatal("unknown key prot remained live after failed reload")
	}
}

type lazyInitRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestConcurrentLazyInitIsSerialized(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &lazyInitRegressionConfig{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cfg.Get("port")
		}()
	}
	wg.Wait()
}

func TestRepeatedInitReturnsErrAlreadyInitialized(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &lazyInitRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	err := cfg.Init(cfg, WithoutFlags())
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second Init error = %v, want ErrAlreadyInitialized", err)
	}
}

func TestFailedInitCanBeRetried(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &lazyInitRegressionConfig{}
	if err := cfg.Init(cfg, WithFileConfig("missing-required.json"), WithoutFlags()); err == nil {
		t.Fatal("first Init: expected missing file error, got nil")
	}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("retry Init: %v", err)
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want default 8080 after retry", got)
	}
}

func TestInitRejectsValidatorsForUnknownKeys(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &lazyInitRegressionConfig{}
	err := cfg.Init(cfg, WithValidation("prot", Required()), WithoutFlags())
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Init error = %v, want ErrUnknownKey", err)
	}
	if !strings.Contains(err.Error(), "prot") {
		t.Fatalf("Init error = %q, want unknown validator key", err.Error())
	}
	if !strings.Contains(err.Error(), "did you mean port?") {
		t.Fatalf("Init error = %q, want key suggestion", err.Error())
	}
}

type strictNumericRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"8080"`
}

func TestStrictJSONLoadRejectsLossyNumericConversion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &strictNumericRegressionConfig{}
	err := cfg.Init(cfg, WithConfigHandler(&memHandler{data: json.RawMessage(`{"port": 8080.5}`)}), WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected lossy numeric conversion error, got nil")
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d, want default retained after failed load", got)
	}
}

func TestLoadConversionErrorIncludesKeyAndSource(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &strictNumericRegressionConfig{}
	err := cfg.Init(cfg, WithConfigHandler(&memHandler{data: json.RawMessage(`{"port": "not-a-number"}`)}), WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected conversion error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{`key "port"`, "from file"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestInitRejectsInvalidEnvValue(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("PORT", "not-a-number")

	cfg := &strictNumericRegressionConfig{}
	err := cfg.Init(cfg, WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected env conversion error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{`key "port"`, "from env PORT"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

type invalidDefaultRegressionConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"not-a-number"`
}

func TestInitRejectsInvalidDefaultTag(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &invalidDefaultRegressionConfig{}
	err := cfg.Init(cfg, WithoutFlags())
	if err == nil {
		t.Fatal("Init: expected invalid default tag error, got nil")
	}
	if !strings.Contains(err.Error(), `invalid default value for key "port"`) {
		t.Fatalf("Init error = %q, want invalid default key context", err.Error())
	}
}

type nonAccessorStrictRegressionConfig struct {
	Structure
	Port int `json:"port"`
}

func TestStrictKeysRejectsNonAccessorField(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &nonAccessorStrictRegressionConfig{}
	err := cfg.Init(cfg,
		WithConfigHandler(&memHandler{data: json.RawMessage(`{"port":8080}`)}),
		WithStrictKeys(),
		WithoutFlags(),
	)
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Init error = %v, want ErrUnknownKey", err)
	}
}

type pointerTextDefaultRegressionValue struct {
	Value int
}

func (v *pointerTextDefaultRegressionValue) UnmarshalText(b []byte) error {
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}
	v.Value = n
	return nil
}

type pointerTextDefaultRegressionConfig struct {
	Structure
	Custom func() *pointerTextDefaultRegressionValue `cfggo:"custom" default:"42"`
}

func TestInitAppliesPointerTextUnmarshalerDefault(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &pointerTextDefaultRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Custom(); got == nil || got.Value != 42 {
		t.Fatalf("Custom() = %#v, want Value 42", got)
	}
}

func TestInitRejectsNilParent(t *testing.T) {
	var cfg Structure
	if err := cfg.Init(nil); err == nil {
		t.Fatal("Init(nil): expected error, got nil")
	}

	var typedNil *lazyInitRegressionConfig
	if err := cfg.Init(typedNil); err == nil {
		t.Fatal("Init((*Config)(nil)): expected error, got nil")
	}
}

func TestPackageInitRejectsTypedNilParent(t *testing.T) {
	var typedNil *lazyInitRegressionConfig
	if err := Init(typedNil); err == nil {
		t.Fatal("cfggo.Init((*Config)(nil)): expected error, got nil")
	}
}

func TestInitRejectsNonPointerParent(t *testing.T) {
	var cfg Structure
	parent := struct {
		Port func() int `cfggo:"port"`
	}{}
	if err := cfg.Init(parent); err == nil {
		t.Fatal("Init(struct{}): expected error, got nil")
	}
}

func TestInitLoadErrorLoggingAvoidsRepeatedSourceContext(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	var logs bytes.Buffer
	cfg := &loadFailureRegressionConfig{}
	err := cfg.Init(cfg,
		WithFileConfig(t.TempDir()),
		WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs)),
		WithoutFlags(),
	)
	if err == nil {
		t.Fatal("Init: expected directory read error, got nil")
	}
	if !errors.Is(err, ErrSource) {
		t.Fatalf("Init error does not match ErrSource: %v", err)
	}

	msg := err.Error()
	for _, repeated := range []string{"cfggo: configuration source error", "failed to load configuration source"} {
		if strings.Contains(msg, repeated) {
			t.Fatalf("error %q contains repeated context %q", msg, repeated)
		}
	}
	for _, want := range []string{"failed to load configuration from file source", "is a directory"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}

	logLine := logs.String()
	if !strings.Contains(logLine, `msg="cfggo: Init failed"`) {
		t.Fatalf("log line %q does not contain concise init failure message", logLine)
	}
	if strings.Contains(logLine, "failed to load configuration source") ||
		strings.Contains(logLine, "cfggo: configuration source error") {
		t.Fatalf("log line %q repeats source context", logLine)
	}
	if strings.Count(logLine, "failed to load configuration from file source") != 1 {
		t.Fatalf("log line %q should include detailed load context exactly once", logLine)
	}
}

func TestInitLoadErrorWithPrintfStyleLoggerAdapter(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	var logs bytes.Buffer
	cfg := &loadFailureRegressionConfig{}
	err := cfg.Init(cfg,
		WithFileConfig(t.TempDir()),
		WithLogger(cfglogger.Plain(&printfStyleRegressionLogger{out: &logs})),
		WithoutFlags(),
	)
	if err == nil {
		t.Fatal("Init: expected directory read error, got nil")
	}

	logLine := logs.String()
	if strings.Contains(logLine, "%!(EXTRA") {
		t.Fatalf("printf-style logger output contains fmt EXTRA noise: %q", logLine)
	}
	for _, want := range []string{
		"cfggo: Init failed",
		"config=loadFailureRegressionConfig",
		`err="[400] failed to load configuration from file source:`,
		"is a directory",
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line %q does not contain %q", logLine, want)
		}
	}
	if strings.Count(logLine, "failed to load configuration from file source") != 1 {
		t.Fatalf("log line %q should include detailed load context exactly once", logLine)
	}
}

type blockingSaveHandler struct {
	data        json.RawMessage
	saveStarted chan struct{}
	releaseSave chan struct{}
	mu          sync.Mutex
	saves       int
}

func (h *blockingSaveHandler) IsDefault() bool { return false }

func (h *blockingSaveHandler) LoadConfig() (json.RawMessage, error) {
	return h.data, nil
}

func (h *blockingSaveHandler) SaveConfig(data json.RawMessage) error {
	select {
	case h.saveStarted <- struct{}{}:
	default:
	}
	<-h.releaseSave
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = data
	h.saves++
	return nil
}

func (h *blockingSaveHandler) saveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saves
}

func TestSaveIfChangedPreservesConcurrentSet(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &blockingSaveHandler{
		data:        json.RawMessage(`{}`),
		saveStarted: make(chan struct{}, 2),
		releaseSave: make(chan struct{}),
	}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("host", "first"); err != nil {
		t.Fatalf("Set first: %v", err)
	}

	saveDone := make(chan error, 1)
	go func() { saveDone <- cfg.SaveIfChanged() }()
	<-handler.saveStarted

	if err := cfg.Set("host", "second"); err != nil {
		t.Fatalf("Set second: %v", err)
	}
	close(handler.releaseSave)
	if err := <-saveDone; err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("second SaveIfChanged: %v", err)
	}
	if got := handler.saveCount(); got != 2 {
		t.Fatalf("SaveConfig calls = %d, want 2", got)
	}
}

type failingReloadHandler struct {
	data  json.RawMessage
	fail  bool
	mu    sync.Mutex
	saves int
}

func (h *failingReloadHandler) IsDefault() bool { return false }

func (h *failingReloadHandler) LoadConfig() (json.RawMessage, error) {
	if h.fail {
		return nil, errors.New("reload failed")
	}
	return h.data, nil
}

func (h *failingReloadHandler) SaveConfig(data json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = data
	h.saves++
	return nil
}

func (h *failingReloadHandler) saveCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.saves
}

func TestFailedReloadPreservesDirtyState(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	handler := &failingReloadHandler{data: json.RawMessage(`{"host":"fromfile"}`)}
	cfg := &reloadDefaultsRegressionConfig{}
	if err := cfg.Init(cfg, WithConfigHandler(handler), WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cfg.Set("host", "runtime"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	handler.fail = true
	if err := cfg.Reload(); err == nil {
		t.Fatal("Reload: expected error, got nil")
	}
	if err := cfg.SaveIfChanged(); err != nil {
		t.Fatalf("SaveIfChanged: %v", err)
	}
	if got := handler.saveCount(); got != 1 {
		t.Fatalf("SaveConfig calls = %d, want 1", got)
	}
}

type explainKeyRegressionConfig struct {
	Structure
	Port   func() int    `cfggo:"port" default:"8080" help:"HTTP listen port"`
	APIKey func() string `cfggo:"api_key" default:"dev-secret" secret:"true" help:"API key"`
}

func TestExplainKeyIncludesFocusedDebugContext(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}
	t.Setenv("PORT", "9090")

	cfg := &explainKeyRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	out := cfg.ExplainKey("port")
	for _, want := range []string{
		"port:",
		"value: 9090",
		"type: int",
		"source: env",
		"source_chain: default->env",
		"env: PORT",
		"default: 8080",
		"help: HTTP listen port",
		"status: ok",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ExplainKey(port) = %q, missing %q", out, want)
		}
	}
}

func TestExplainKeyMasksSecretsAndReportsUnknownKey(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &explainKeyRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	out := cfg.ExplainKey("api_key")
	if !strings.Contains(out, "value: ****") || !strings.Contains(out, "default: ****") {
		t.Fatalf("ExplainKey(api_key) = %q, want masked value and default", out)
	}
	if strings.Contains(out, "dev-secret") {
		t.Fatalf("ExplainKey(api_key) leaked secret default: %q", out)
	}

	if got := cfg.ExplainKey("missing"); !strings.Contains(got, "missing: <unknown key>") {
		t.Fatalf("ExplainKey(missing) = %q, want unknown-key explanation", got)
	}
}

type suggestionRegressionConfig struct {
	Structure
	ServerPort func() int `cfggo:"server_port" default:"8080"`
}

func TestUnknownLoadedKeyIncludesSuggestion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &suggestionRegressionConfig{}
	err := cfg.Init(cfg,
		WithConfigHandler(&memHandler{data: json.RawMessage(`{"server_prt":9090}`)}),
		WithStrictKeys(),
		WithoutFlags(),
	)
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Init error = %v, want ErrUnknownKey", err)
	}
	if !strings.Contains(err.Error(), "server_prt (did you mean server_port?)") {
		t.Fatalf("Init error = %q, want unknown-key suggestion", err.Error())
	}
}

func TestDiagnosticsIncludeUnknownKeySuggestion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &suggestionRegressionConfig{}
	if err := cfg.Init(cfg,
		WithConfigHandler(&memHandler{data: json.RawMessage(`{"server_prt":9090}`)}),
		WithoutFlags(),
	); err != nil {
		t.Fatalf("Init: %v", err)
	}

	diag := cfg.DiagnoseData()
	var found bool
	for _, key := range diag.Keys {
		if key.Key == "server_prt" {
			found = true
			if key.Suggestion != "server_port" {
				t.Fatalf("Suggestion = %q, want server_port", key.Suggestion)
			}
		}
	}
	if !found {
		t.Fatal("DiagnoseData missing unrecognized server_prt key")
	}
	if out := diag.String(); !strings.Contains(out, "unrecognized (did you mean server_port?)") {
		t.Fatalf("Diagnostics.String() = %q, want suggestion", out)
	}
}

func TestRuntimeUnknownKeyErrorsIncludeSuggestion(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &suggestionRegressionConfig{}
	if err := cfg.Init(cfg, WithoutFlags()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	for label, err := range map[string]error{
		"Set":         cfg.Set("server_prt", 9090),
		"ValidateKey": cfg.ValidateKey("server_prt"),
	} {
		if !errors.Is(err, ErrUnknownKey) {
			t.Fatalf("%s error = %v, want ErrUnknownKey", label, err)
		}
		if !strings.Contains(err.Error(), `did you mean "server_port"?`) {
			t.Fatalf("%s error = %q, want suggestion", label, err.Error())
		}
	}

	if out := cfg.ExplainKey("server_prt"); !strings.Contains(out, `did you mean "server_port"?`) {
		t.Fatalf("ExplainKey(server_prt) = %q, want suggestion", out)
	}
}

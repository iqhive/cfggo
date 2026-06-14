package cfggo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/errwrapper"
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

func TestErrwrapperPackageAliasesCfgerror(t *testing.T) {
	err := GlobalErrorWrapper()(errors.New("cause"), ErrCodeInvalidArgument, "wrapped")

	var modern *cfgerror.Error
	if !errors.As(err, &modern) {
		t.Fatalf("errors.As(*cfgerror.Error) = false for %T: %v", err, err)
	}
	var legacy *errwrapper.Error
	if !errors.As(err, &legacy) {
		t.Fatalf("errors.As(*errwrapper.Error) = false for %T: %v", err, err)
	}
	if modern != legacy {
		t.Fatalf("cfgerror and errwrapper aliases resolved to different errors: %p vs %p", modern, legacy)
	}
	if got, want := errwrapper.Code(err), cfgerror.Code(err); got != want {
		t.Fatalf("errwrapper.Code = %d, cfgerror.Code = %d", got, want)
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

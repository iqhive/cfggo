package cfggo

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/cfgerror"
	"github.com/iqhive/cfggo/cfglogger"
	"github.com/iqhive/cfggo/sources"
	"github.com/iqhive/cfggo/validcfg"
)

type coverageConfig struct {
	Structure
	Port   func() int      `cfggo:"port" help:"listen port"`
	Name   func() string   `cfggo:"name" help:"service name"`
	Secret func() string   `cfggo:"secret" secret:"true"`
	Labels func() []string `cfggo:"labels"`
}

func newCoverageConfig() *coverageConfig {
	return &coverageConfig{
		Port:   DefaultValue(8080),
		Name:   DefaultValue("api"),
		Secret: DefaultValue("shh"),
		Labels: DefaultClone([]string{"base"}),
	}
}

type captureHandler struct {
	loadData json.RawMessage
	saved    json.RawMessage
}

func (h *captureHandler) IsDefault() bool { return false }
func (h *captureHandler) LoadConfig() (json.RawMessage, error) {
	return h.loadData, nil
}
func (h *captureHandler) SaveConfig(data json.RawMessage) error {
	h.saved = append(h.saved[:0], data...)
	return nil
}

func TestStructureConvenienceMethodsAndDiagnostics(t *testing.T) {
	handler := &captureHandler{}
	cfg := newCoverageConfig()
	if err := cfg.Init(
		cfg,
		WithName("coverage"),
		WithoutFlags(),
		WithoutEnv(),
		WithConfigHandler(handler),
		WithValidation("port", Range(1, 9000)),
	); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := (&Structure{}).InitSelf(WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("InitSelf() on bare Structure error = %v", err)
	}

	if err := cfg.Set("name", "worker"); err != nil {
		t.Fatalf("Set(name) error = %v", err)
	}
	if err := cfg.Set("labels", []string{"blue"}); err != nil {
		t.Fatalf("Set(labels) error = %v", err)
	}

	if got, ok := Value[string](&cfg.Structure, "name"); !ok || got != "worker" {
		t.Fatalf("Value[string](name) = %q, %v; want worker, true", got, ok)
	}
	labels, ok := Value[[]string](&cfg.Structure, "labels")
	if !ok || len(labels) != 1 || labels[0] != "blue" {
		t.Fatalf("Value[[]string](labels) = %#v, %v; want [blue], true", labels, ok)
	}
	labels[0] = "mutated"
	if got := MustValue[[]string](&cfg.Structure, "labels"); got[0] != "blue" {
		t.Fatalf("Value should return a clone for slices; got %#v", got)
	}
	if got := MustValue[int](&cfg.Structure, "missing"); got != 0 {
		t.Fatalf("MustValue(missing) = %d, want zero", got)
	}

	data, err := cfg.GetJSONBytes()
	if err != nil {
		t.Fatalf("GetJSONBytes() error = %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(GetJSONBytes()) error = %v", err)
	}
	if decoded["name"] != "worker" {
		t.Fatalf("JSON name = %#v, want worker", decoded["name"])
	}

	for label, rendered := range map[string]string{
		"String":          cfg.String(),
		"Explain":         cfg.Explain(),
		"ConfigReference": cfg.ConfigReference(),
		"Report":          cfg.Report(),
	} {
		if !strings.Contains(rendered, "name") {
			t.Fatalf("%s output = %q, want key name", label, rendered)
		}
		if strings.Contains(rendered, "shh") {
			t.Fatalf("%s output leaked secret value: %q", label, rendered)
		}
	}

	diag := cfg.Diagnose()
	if diag.Name != "coverage" || !diag.Valid {
		t.Fatalf("Diagnose() = %#v, want coverage and valid", diag)
	}
	if reference := diag.Reference(); !strings.Contains(reference, "PORT") || !strings.Contains(reference, "listen port") {
		t.Fatalf("Diagnostics.Reference() = %q, want env/help details", reference)
	}

	srcs := cfg.Sources()
	if srcs["name"] != SourceSet {
		t.Fatalf("Sources()[name] = %v, want set", srcs["name"])
	}
	srcs["name"] = SourceHTTP
	if src, _ := cfg.Source("name"); src != SourceSet {
		t.Fatalf("Sources() should return a copy; source = %v, want set", src)
	}
	chain := cfg.SourceChain("name")
	if len(chain) != 2 || chain[0] != SourceDefault || chain[1] != SourceSet {
		t.Fatalf("SourceChain(name) = %#v, want default -> set", chain)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if !bytes.Contains(handler.saved, []byte(`"name":"worker"`)) {
		t.Fatalf("saved JSON = %s, want name", string(handler.saved))
	}

	cfg.AddValidator("name", MinLength(3))
	if err := cfg.ValidateKey("name"); err != nil {
		t.Fatalf("ValidateKey(name) error = %v", err)
	}
	if err := cfg.Set("port", 0); err != nil {
		t.Fatalf("Set(port) error = %v", err)
	}
	if err := cfg.ValidateKey("port"); !errors.Is(err, validcfg.ErrValidation) {
		t.Fatalf("ValidateKey(port) error = %v, want ErrValidation", err)
	}

	logger := &cfglogger.NoopLogger{}
	cfg.SetLogger(logger)
	if cfg.GetLogger() != logger {
		t.Fatal("GetLogger() should return instance logger")
	}
	cfg.SetErrorWrapper(cfgerror.NewDefaultWrapper())
	if err := cfg.WrapError(errors.New("boom"), 123, "wrapped"); cfgerror.Code(err) != 123 {
		t.Fatalf("WrapError() = %v, want code 123", err)
	}

	CleanupSignalHandler()
}

func TestOptionSettersAndRootValidators(t *testing.T) {
	s := &Structure{}
	if err := withNoop()(s); err != nil {
		t.Fatalf("withNoop() error = %v", err)
	}
	if err := WithoutEnv()(s); err != nil {
		t.Fatalf("WithoutEnv() error = %v", err)
	}
	if !s.skipEnv {
		t.Fatal("WithoutEnv() did not set skipEnv")
	}
	if err := WithSkipEnvironment()(s); err != nil {
		t.Fatalf("WithSkipEnvironment() error = %v", err)
	}

	s.configHandler = sources.NewHandlerEnv("OLD_", false)
	if err := WithEnvConfig()(s); err != nil {
		t.Fatalf("WithEnvConfig() error = %v", err)
	}
	if !s.envPrefixSet || s.envPrefix != "" || s.configHandler != nil {
		t.Fatalf("WithEnvConfig() state = prefix %q set %v handler %#v", s.envPrefix, s.envPrefixSet, s.configHandler)
	}
	if err := WithEnvPrefix("APP_")(s); err != nil {
		t.Fatalf("WithEnvPrefix() error = %v", err)
	}
	if s.envPrefix != "APP_" {
		t.Fatalf("envPrefix = %q, want APP_", s.envPrefix)
	}

	if err := WithFlagSet(nil)(s); err == nil {
		t.Fatal("WithFlagSet(nil): expected error, got nil")
	}
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	if err := WithFlagSet(fs)(s); err != nil {
		t.Fatalf("WithFlagSet() error = %v", err)
	}
	if s.FlagSet != fs || !s.externalFlagSet {
		t.Fatal("WithFlagSet() did not store external flag set")
	}

	if err := WithoutFlags()(s); err != nil {
		t.Fatalf("WithoutFlags() error = %v", err)
	}
	if !s.noFlags {
		t.Fatal("WithoutFlags() did not set noFlags")
	}
	if err := WithIgnoreUnknownVars()(s); err != nil {
		t.Fatalf("WithIgnoreUnknownVars() error = %v", err)
	}
	if !s.ignoreUnknownVars {
		t.Fatal("WithIgnoreUnknownVars() did not set ignoreUnknownVars")
	}
	if err := WithLenientLoad()(s); err != nil {
		t.Fatalf("WithLenientLoad() error = %v", err)
	}
	if !s.lenient {
		t.Fatal("WithLenientLoad() did not set lenient")
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.test/config", nil)
	s.configHandler = nil
	if err := WithHTTPConfig(nil, nil)(s); err == nil {
		t.Fatal("WithHTTPConfig(nil, nil): expected error, got nil")
	}
	if err := WithHTTPConfig(req, nil)(s); err != nil {
		t.Fatalf("WithHTTPConfig() error = %v", err)
	}
	if _, ok := s.configHandler.(*sources.HandlerHTTP); !ok {
		t.Fatalf("configHandler = %T, want HandlerHTTP", s.configHandler)
	}
	if err := WithHTTPConfig(req, nil)(s); err == nil {
		t.Fatal("WithHTTPConfig() with existing handler: expected error, got nil")
	}

	wrapper := func(err error, code int, msg string, args ...interface{}) error {
		return &cfgerror.Error{Code: code, Msg: msg, Err: err}
	}
	if err := WithErrorWrapper(wrapper)(s); err != nil {
		t.Fatalf("WithErrorWrapper() error = %v", err)
	}
	if err := s.WrapError(errors.New("boom"), 77, "custom"); cfgerror.Code(err) != 77 {
		t.Fatalf("custom WrapError() = %v, want code 77", err)
	}

	rootValidators := []validcfg.Validator{
		Required(),
		MinLength(1),
		MaxLength(10),
		Range(1, 10),
		OneOf("value"),
		Regex(`^value$`),
		Email(),
		URL(),
		All(Required()),
		Any(Required()),
		Custom(func(interface{}) error { return nil }),
	}
	values := []interface{}{
		"value",
		"value",
		"value",
		5,
		"value",
		"value",
		"user@example.com",
		"https://example.com",
		"value",
		"value",
		"value",
	}
	for i, validator := range rootValidators {
		if err := validator(values[i]); err != nil {
			t.Fatalf("root validator %d error = %v", i, err)
		}
	}
}

func TestGlobalLoggingHelpers(t *testing.T) {
	oldLogger := GlobalLogger()
	oldWrapper := GlobalErrorWrapper()
	oldLevel := currentLevel.Load()
	t.Cleanup(func() {
		SetGlobalLogger(oldLogger)
		SetGlobalErrorWrapper(oldWrapper)
		currentLevel.Store(oldLevel)
	})

	for input, want := range map[string]LogLevel{
		"debug":   LogLevelDebug,
		" INFO ":  LogLevelInfo,
		"warning": LogLevelWarn,
		"error":   LogLevelError,
		"off":     LogLevelNone,
	} {
		got, ok := ParseLogLevel(input)
		if !ok || got != want {
			t.Fatalf("ParseLogLevel(%q) = %v, %v; want %v, true", input, got, ok, want)
		}
	}
	if got, ok := ParseLogLevel("loud"); ok || got != LogLevelInfo {
		t.Fatalf("ParseLogLevel(loud) = %v, %v; want info, false", got, ok)
	}
	if got := LogLevel(999).slogLevel(); got.String() != "INFO" {
		t.Fatalf("unknown slogLevel() = %v, want info", got)
	}

	var logs bytes.Buffer
	SetLogOutput(&logs)
	SetLogLevel(LogLevelWarn)
	GlobalLogger().Info("hidden")
	GlobalLogger().Warn("visible")
	if got := logs.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "visible") {
		t.Fatalf("warn-level logs = %q, want visible warn only", got)
	}

	SetLogLevel(LogLevelDebug)
	GlobalLogger().Debug("debug-visible")
	if got := logs.String(); !strings.Contains(got, "debug-visible") {
		t.Fatalf("debug logs = %q, want debug-visible", got)
	}

	SetGlobalLogger(nil)
	if GlobalLogger() == nil {
		t.Fatal("SetGlobalLogger(nil) should not clear logger")
	}
	SetGlobalErrorWrapper(nil)
	if GlobalErrorWrapper() == nil {
		t.Fatal("SetGlobalErrorWrapper(nil) should not clear wrapper")
	}
}

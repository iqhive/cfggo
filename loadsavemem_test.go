package cfggo

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/sources"
)

// memHandler is a minimal in-memory ConfigHandler for tests.
type memHandler struct{ data json.RawMessage }

func (h *memHandler) IsDefault() bool                      { return false }
func (h *memHandler) LoadConfig() (json.RawMessage, error) { return h.data, nil }
func (h *memHandler) SaveConfig(b json.RawMessage) error   { h.data = b; return nil }

var _ sources.ConfigHandler = (*memHandler)(nil)

type chainConfig struct {
	Structure
	Name func() string `cfggo:"name" default:"defname" help:"the name"`
	Port func() int    `cfggo:"port"`
}

func initChainConfig(t *testing.T, opts ...Option) *chainConfig {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"test"}

	cfg := &chainConfig{Name: DefaultValue("defname"), Port: DefaultValue(0)}
	fs := flag.NewFlagSet("chain", flag.ContinueOnError)
	all := append([]Option{WithName("chain"), WithFlagSet(fs)}, opts...)
	if err := cfg.Init(cfg, all...); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cfg
}

// SourceChain records the ordered override chain and Explain surfaces it.
func TestSourceChainAndExplain(t *testing.T) {
	t.Setenv("PORT", "9090")
	cfg := initChainConfig(t)

	// port: default (applyPlan) then env override -> chain [default, env].
	chain := cfg.SourceChain("port")
	if len(chain) != 2 || chain[0] != SourceDefault || chain[1] != SourceEnv {
		t.Fatalf("port SourceChain = %v, want [default env]", chain)
	}

	if err := cfg.Set("name", "override"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if chain := cfg.SourceChain("name"); len(chain) != 2 || chain[0] != SourceDefault || chain[1] != SourceSet {
		t.Fatalf("name SourceChain = %v, want [default set]", chain)
	}

	// A single-source key reports a one-element chain.
	cfg2 := initChainConfig(t)
	if chain := cfg2.SourceChain("name"); len(chain) != 1 || chain[0] != SourceDefault {
		t.Fatalf("unoverridden name SourceChain = %v, want [default]", chain)
	}

	exp := cfg.Explain()
	if !strings.Contains(exp, "[default->env]") {
		t.Errorf("Explain() missing override chain for port:\n%s", exp)
	}
}

// OnChange delivers the key, old/new values, and source.
func TestOnChangeCarriesValuesAndSource(t *testing.T) {
	cfg := initChainConfig(t)
	var got []Change
	cancel := cfg.OnChange(func(changes []Change) { got = append(got, changes...) })
	defer cancel()

	if err := cfg.Set("port", 4242); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
	ch := got[0]
	if ch.Key != "port" || ch.Old != 0 || ch.New != 4242 || ch.Source != SourceSet {
		t.Errorf("change = %+v, want {port 0 4242 set}", ch)
	}
}

// WithIgnoreKeys exempts intentional unknown keys from the unrecognized report.
func TestWithIgnoreKeys(t *testing.T) {
	handler := &memHandler{data: json.RawMessage(`{"name":"x","port":1,"trace_id":"abc"}`)}

	cfgWith := initChainConfig(t, WithConfigHandler(handler), WithIgnoreKeys("trace_id"))
	for _, k := range cfgWith.DiagnoseData().Unrecognized {
		if k == "trace_id" {
			t.Fatalf("trace_id should be exempt, got unrecognized %v", cfgWith.DiagnoseData().Unrecognized)
		}
	}

	// Without the exemption it is reported as unrecognized.
	handler2 := &memHandler{data: json.RawMessage(`{"name":"x","port":1,"trace_id":"abc"}`)}
	cfgWithout := initChainConfig(t, WithConfigHandler(handler2))
	found := false
	for _, k := range cfgWithout.DiagnoseData().Unrecognized {
		if k == "trace_id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("trace_id should be unrecognized without WithIgnoreKeys")
	}
}

// DiagnoseData populates the static reference fields used by ConfigReference.
func TestDiagnoseDataFields(t *testing.T) {
	cfg := initChainConfig(t)
	d := cfg.DiagnoseData()
	var name KeyDiagnostic
	for _, k := range d.Keys {
		if k.Key == "name" {
			name = k
		}
	}
	if !name.IsAccessor || !name.Recognized {
		t.Errorf("name diagnostic: IsAccessor=%v Recognized=%v, want true/true", name.IsAccessor, name.Recognized)
	}
	if !name.HasDefault || name.Default != "defname" {
		t.Errorf("name default = %q (has=%v), want \"defname\"", name.Default, name.HasDefault)
	}
	if name.Help != "the name" {
		t.Errorf("name help = %q, want \"the name\"", name.Help)
	}
}

// The package-level Init errors when the target does not embed Structure.
func TestPackageInitRequiresStructure(t *testing.T) {
	type notAConfig struct {
		Port int
	}
	err := Init(&notAConfig{})
	if err == nil {
		t.Fatal("Init on a non-Structure type: expected error, got nil")
	}
	if code := ErrorCode(err); code != ErrCodeInvalidArgument {
		t.Errorf("ErrorCode = %d, want %d; err = %v", code, ErrCodeInvalidArgument, err)
	}
}

package cfggo

import (
	"testing"
)

type cloneOptions struct {
	Hosts  []string
	Meta   map[string]string
	Nested struct {
		Ports []int
	}
	hidden []string // unexported fields are copied as-is (reflection cannot assign them)
}

type cloneConfig struct {
	Structure
	Labels func() map[string]interface{} `cfggo:"labels"`
	Items  func() []interface{}          `cfggo:"items"`
	Opts   func() cloneOptions           `cfggo:"opts"`
	Tags   func() map[string]string      `cfggo:"tags"`
}

func newCloneConfig(t *testing.T) *cloneConfig {
	t.Helper()
	opts := cloneOptions{Hosts: []string{"h1"}, Meta: map[string]string{"m": "1"}, hidden: []string{"h"}}
	opts.Nested.Ports = []int{1}
	cfg := &cloneConfig{
		Labels: DefaultValue(map[string]interface{}{
			"a":    map[string]interface{}{"x": 1},
			"list": []interface{}{"p"},
		}),
		Items: DefaultValue([]interface{}{[]interface{}{"q"}, map[string]interface{}{"k": "v"}}),
		Opts:  DefaultValue(opts),
		Tags:  DefaultValue(map[string]string{"k": "v"}),
	}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cfg
}

func (c *cloneConfig) assertPristine(t *testing.T, when string) {
	t.Helper()
	labels := c.Labels()
	if got := labels["a"].(map[string]interface{})["x"]; got != 1 {
		t.Errorf("%s: Labels()[a][x] = %v, want 1 (nested map aliased)", when, got)
	}
	if got := labels["list"].([]interface{})[0]; got != "p" {
		t.Errorf("%s: Labels()[list][0] = %v, want p (nested slice aliased)", when, got)
	}
	items := c.Items()
	if got := items[0].([]interface{})[0]; got != "q" {
		t.Errorf("%s: Items()[0][0] = %v, want q", when, got)
	}
	if got := items[1].(map[string]interface{})["k"]; got != "v" {
		t.Errorf("%s: Items()[1][k] = %v, want v", when, got)
	}
	opts := c.Opts()
	if opts.Hosts[0] != "h1" || opts.Meta["m"] != "1" || opts.Nested.Ports[0] != 1 || len(opts.hidden) != 1 {
		t.Errorf("%s: Opts() = %+v, want the original struct contents", when, opts)
	}
	if got := c.Tags()["k"]; got != "v" {
		t.Errorf("%s: Tags()[k] = %v, want v", when, got)
	}
}

func mutateEverything(labels map[string]interface{}, items []interface{}, opts cloneOptions, tags map[string]string) {
	labels["a"].(map[string]interface{})["x"] = 99
	labels["list"].([]interface{})[0] = "MUTATED"
	items[0].([]interface{})[0] = "MUTATED"
	items[1].(map[string]interface{})["k"] = "MUTATED"
	opts.Hosts[0] = "MUTATED"
	opts.Meta["m"] = "MUTATED"
	opts.Nested.Ports[0] = 99
	tags["k"] = "MUTATED"
}

func TestAccessorCopiesDoNotAliasNestedContainers(t *testing.T) {
	cfg := newCloneConfig(t)
	mutateEverything(cfg.Labels(), cfg.Items(), cfg.Opts(), cfg.Tags())
	cfg.assertPristine(t, "after mutating accessor results")
}

func TestGetAndValueCopiesDoNotAliasNestedContainers(t *testing.T) {
	cfg := newCloneConfig(t)
	labels, _ := cfg.Get("labels")
	items, _ := cfg.Get("items")
	opts := MustValue[cloneOptions](&cfg.Structure, "opts")
	tags := MustValue[map[string]string](&cfg.Structure, "tags")
	mutateEverything(labels.(map[string]interface{}), items.([]interface{}), opts, tags)
	cfg.assertPristine(t, "after mutating Get/Value results")
}

func TestDiagnoseDataValuesDoNotAliasLiveConfig(t *testing.T) {
	cfg := newCloneConfig(t)
	d := cfg.DiagnoseData()
	var labels map[string]interface{}
	var items []interface{}
	var opts cloneOptions
	var tags map[string]string
	for _, k := range d.Keys {
		switch k.Key {
		case "labels":
			labels = k.Value.(map[string]interface{})
		case "items":
			items = k.Value.([]interface{})
		case "opts":
			opts = k.Value.(cloneOptions)
		case "tags":
			tags = k.Value.(map[string]string)
		}
	}
	mutateEverything(labels, items, opts, tags)
	cfg.assertPristine(t, "after mutating DiagnoseData values")
}

func TestReloadRestoresDefaultsUntouchedByCallerMutation(t *testing.T) {
	cfg := newCloneConfig(t)
	// Legitimately change a value, then corrupt every copy a caller can reach
	if err := cfg.Set("tags", map[string]string{"k": "changed"}); err != nil {
		t.Fatal(err)
	}
	mutateEverything(cfg.Labels(), cfg.Items(), cfg.Opts(), cfg.Tags())
	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	// Set survives a reload (runtime override) but the defaults snapshot used
	// for every other key must be the original, unmutated values
	if got := cfg.Tags()["k"]; got != "changed" {
		t.Errorf("Tags()[k] after reload = %q, want the Set value to be re-asserted", got)
	}
	labels := cfg.Labels()
	if got := labels["a"].(map[string]interface{})["x"]; got != 1 {
		t.Errorf("Labels()[a][x] after reload = %v, want 1", got)
	}
	if got := cfg.Opts().Hosts[0]; got != "h1" {
		t.Errorf("Opts().Hosts[0] after reload = %q, want h1", got)
	}
}

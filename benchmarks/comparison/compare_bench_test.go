// Package comparison benchmarks cfggo against three popular Go configuration
// libraries — Viper, koanf, and envconfig — on the two most directly comparable
// operations: loading a configuration and reading a typed value back out.
//
// Caveats to read the numbers fairly:
//
//   - cfggo, Viper, and koanf all parse the SAME in-memory JSON document, so
//     their Load benchmarks are apples-to-apples.
//   - envconfig has no notion of a config file; it only reads environment
//     variables into a struct. Its Load benchmark therefore measures
//     env-variable parsing, and its "read" is a plain Go struct field access
//     (effectively free) — included for completeness, not as a like-for-like
//     comparison.
//   - cfggo and Viper/koanf expose values through a live map keyed by string,
//     so reads have real cost; envconfig resolves everything at Process() time.
package comparison

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/viper"

	"github.com/iqhive/cfggo"
)

// configJSON is the shared document parsed by cfggo, Viper, and koanf.
var configJSON = []byte(`{"port":8080,"name":"service","debug":true,"timeout":"30s","rate":1.5}`)

// envForConfig mirrors configJSON as environment variables for envconfig.
var envForConfig = map[string]string{
	"PORT":    "8080",
	"NAME":    "service",
	"DEBUG":   "true",
	"TIMEOUT": "30s",
	"RATE":    "1.5",
}

func TestMain(m *testing.M) {
	cfggo.SetLogLevel(cfggo.LogLevelNone)
	for k, v := range envForConfig {
		os.Setenv(k, v)
	}
	code := m.Run()
	for k := range envForConfig {
		os.Unsetenv(k)
	}
	os.Exit(code)
}

// --- cfggo ------------------------------------------------------------------

type cfggoConfig struct {
	cfggo.Structure

	Port    func() int           `cfggo:"port"    default:"0"`
	Name    func() string        `cfggo:"name"    default:""`
	Debug   func() bool          `cfggo:"debug"   default:"false"`
	Timeout func() time.Duration `cfggo:"timeout" default:"0s"`
	Rate    func() float64       `cfggo:"rate"    default:"0"`
}

// memHandler feeds cfggo the in-memory JSON document, the analogue of Viper's
// ReadConfig(bytes) and koanf's rawbytes provider.
type memHandler struct{ data json.RawMessage }

func (h memHandler) IsDefault() bool                      { return false }
func (h memHandler) LoadConfig() (json.RawMessage, error) { return h.data, nil }
func (h memHandler) SaveConfig(json.RawMessage) error     { return nil }

func loadCfggo(b *testing.B) *cfggoConfig {
	cfg := &cfggoConfig{}
	if err := cfggo.Init(cfg,
		cfggo.WithSkipEnvironment(),
		cfggo.WithConfigHandler(memHandler{data: configJSON}),
	); err != nil {
		b.Fatalf("cfggo init: %v", err)
	}
	return cfg
}

// --- envconfig --------------------------------------------------------------

type envConfig struct {
	Port    int           `envconfig:"PORT"`
	Name    string        `envconfig:"NAME"`
	Debug   bool          `envconfig:"DEBUG"`
	Timeout time.Duration `envconfig:"TIMEOUT"`
	Rate    float64       `envconfig:"RATE"`
}

// --- koanf ------------------------------------------------------------------

func loadKoanf(b *testing.B) *koanf.Koanf {
	k := koanf.New(".")
	if err := k.Load(rawbytes.Provider(configJSON), kjson.Parser()); err != nil {
		b.Fatalf("koanf load: %v", err)
	}
	return k
}

// --- Viper ------------------------------------------------------------------

func loadViper(b *testing.B) *viper.Viper {
	v := viper.New()
	v.SetConfigType("json")
	if err := v.ReadConfig(bytes.NewReader(configJSON)); err != nil {
		b.Fatalf("viper read: %v", err)
	}
	return v
}

// =====================  LOAD / INIT BENCHMARKS  =============================

func BenchmarkLoad_Cfggo(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cfg := &cfggoConfig{}
		if err := cfggo.Init(cfg,
			cfggo.WithSkipEnvironment(),
			cfggo.WithConfigHandler(memHandler{data: configJSON}),
		); err != nil {
			b.Fatal(err)
		}
		_ = cfg
	}
}

func BenchmarkLoad_Viper(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v := viper.New()
		v.SetConfigType("json")
		if err := v.ReadConfig(bytes.NewReader(configJSON)); err != nil {
			b.Fatal(err)
		}
		_ = v
	}
}

func BenchmarkLoad_Koanf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		k := koanf.New(".")
		if err := k.Load(rawbytes.Provider(configJSON), kjson.Parser()); err != nil {
			b.Fatal(err)
		}
		_ = k
	}
}

// BenchmarkLoad_Envconfig measures env-variable parsing into a struct. Unlike
// the others it reads from the environment, not the JSON document.
func BenchmarkLoad_Envconfig(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var c envConfig
		if err := envconfig.Process("", &c); err != nil {
			b.Fatal(err)
		}
		_ = c
	}
}

// =====================  TYPED READ BENCHMARKS  =============================

func BenchmarkReadInt_Cfggo(b *testing.B) {
	cfg := loadCfggo(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = cfg.Port()
	}
	_ = sink
}

func BenchmarkReadInt_Viper(b *testing.B) {
	v := loadViper(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = v.GetInt("port")
	}
	_ = sink
}

func BenchmarkReadInt_Koanf(b *testing.B) {
	k := loadKoanf(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = k.Int("port")
	}
	_ = sink
}

// BenchmarkReadInt_Envconfig is a plain struct field read (resolved at Process
// time), shown for reference — it is essentially free.
func BenchmarkReadInt_Envconfig(b *testing.B) {
	var c envConfig
	if err := envconfig.Process("", &c); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink int
	for i := 0; i < b.N; i++ {
		sink = c.Port
	}
	_ = sink
}

func BenchmarkReadString_Cfggo(b *testing.B) {
	cfg := loadCfggo(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.Name()
	}
	_ = sink
}

func BenchmarkReadString_Viper(b *testing.B) {
	v := loadViper(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = v.GetString("name")
	}
	_ = sink
}

func BenchmarkReadString_Koanf(b *testing.B) {
	k := loadKoanf(b)
	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = k.String("name")
	}
	_ = sink
}

func BenchmarkReadString_Envconfig(b *testing.B) {
	var c envConfig
	if err := envconfig.Process("", &c); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = c.Name
	}
	_ = sink
}

package benchmarks

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/iqhive/cfggo"
)

// TestMain silences cfggo logging for the whole suite so log formatting and I/O
// never skew the measurements, and so benchmark output stays clean.
func TestMain(m *testing.M) {
	cfggo.SetLogLevel(cfggo.LogLevelNone)
	os.Exit(m.Run())
}

// --- Sample configuration structs of varying size --------------------------

// smallConfig is a minimal, realistic service config (5 fields).
type smallConfig struct {
	cfggo.Structure

	Name    func() string        `cfggo:"name"    default:"svc"        help:"service name"`
	Host    func() string        `cfggo:"host"    default:"localhost"  help:"listen host"`
	Port    func() int           `cfggo:"port"    default:"8080"       help:"listen port"`
	Debug   func() bool          `cfggo:"debug"   default:"false"      help:"enable debug mode"`
	Timeout func() time.Duration `cfggo:"timeout" default:"30s"        help:"request timeout"`
}

// mediumConfig spans every common scalar, collection, and time type (16 fields).
type mediumConfig struct {
	cfggo.Structure

	AppName  func() string            `cfggo:"app_name"  default:"svc"            help:"application name"`
	Port     func() int               `cfggo:"port"      default:"8080"           help:"listen port"`
	Debug    func() bool              `cfggo:"debug"     default:"false"          help:"enable debug mode"`
	Timeout  func() time.Duration     `cfggo:"timeout"   default:"30s"            help:"request timeout"`
	MaxConns func() int               `cfggo:"max_conns" default:"100"            help:"max connections"`
	Rate     func() float64           `cfggo:"rate"      default:"1.5"            help:"sample rate"`
	Ratio    func() float32           `cfggo:"ratio"     default:"0.5"            help:"backoff ratio"`
	LogLevel func() string            `cfggo:"log_level" default:"info"           help:"log level"`
	Tags     func() []string          `cfggo:"tags"      default:"a,b,c"          help:"labels"`
	Ports    func() []int             `cfggo:"ports"     default:"1,2,3"          help:"extra ports"`
	Labels   func() map[string]string `cfggo:"labels"    default:"{\"k\":\"v\"}"  help:"string labels"`
	Limits   func() map[string]int    `cfggo:"limits"    default:"{\"cpu\":2}"    help:"resource limits"`
	Retries  func() int               `cfggo:"retries"   default:"3"              help:"retry attempts"`
	Verbose  func() bool              `cfggo:"verbose"   default:"true"           help:"verbose logging"`
	Region   func() string            `cfggo:"region"    default:"us-east-1"      help:"deployment region"`
	Weight   func() float64           `cfggo:"weight"    default:"2.0"            help:"routing weight"`
}

// dbConfig and serverConfig form a two-level nested configuration so the
// benchmarks can measure access through dotted, flattened keys.
type dbConfig struct {
	Host     func() string `cfggo:"host"     default:"localhost" help:"db host"`
	Port     func() int    `cfggo:"port"     default:"5432"      help:"db port"`
	Username func() string `cfggo:"username" default:"user"      help:"db user"`
	Password func() string `cfggo:"password" default:"pass"      help:"db password"`
}

type serverConfig struct {
	Listen func() string `cfggo:"listen" default:":8080" help:"listen address"`
	DB     dbConfig      `cfggo:"db"`
}

type nestedConfig struct {
	cfggo.Structure

	Name   func() string `cfggo:"name" default:"svc" help:"service name"`
	Server serverConfig  `cfggo:"server"`
}

// largeConfig has 40 fields across all supported categories, used to measure
// how Init and the whole-config operations scale with field count.
type largeConfig struct {
	cfggo.Structure

	Int0 func() int `cfggo:"int0" default:"0"`
	Int1 func() int `cfggo:"int1" default:"1"`
	Int2 func() int `cfggo:"int2" default:"2"`
	Int3 func() int `cfggo:"int3" default:"3"`
	Int4 func() int `cfggo:"int4" default:"4"`
	Int5 func() int `cfggo:"int5" default:"5"`
	Int6 func() int `cfggo:"int6" default:"6"`
	Int7 func() int `cfggo:"int7" default:"7"`
	Int8 func() int `cfggo:"int8" default:"8"`
	Int9 func() int `cfggo:"int9" default:"9"`

	Str0 func() string `cfggo:"str0" default:"v0"`
	Str1 func() string `cfggo:"str1" default:"v1"`
	Str2 func() string `cfggo:"str2" default:"v2"`
	Str3 func() string `cfggo:"str3" default:"v3"`
	Str4 func() string `cfggo:"str4" default:"v4"`
	Str5 func() string `cfggo:"str5" default:"v5"`
	Str6 func() string `cfggo:"str6" default:"v6"`
	Str7 func() string `cfggo:"str7" default:"v7"`
	Str8 func() string `cfggo:"str8" default:"v8"`
	Str9 func() string `cfggo:"str9" default:"v9"`

	Bool0 func() bool `cfggo:"bool0" default:"true"`
	Bool1 func() bool `cfggo:"bool1" default:"false"`
	Bool2 func() bool `cfggo:"bool2" default:"true"`
	Bool3 func() bool `cfggo:"bool3" default:"false"`
	Bool4 func() bool `cfggo:"bool4" default:"true"`

	Flt0 func() float64 `cfggo:"flt0" default:"0.5"`
	Flt1 func() float64 `cfggo:"flt1" default:"1.5"`
	Flt2 func() float64 `cfggo:"flt2" default:"2.5"`
	Flt3 func() float64 `cfggo:"flt3" default:"3.5"`
	Flt4 func() float64 `cfggo:"flt4" default:"4.5"`

	Dur0 func() time.Duration `cfggo:"dur0" default:"1s"`
	Dur1 func() time.Duration `cfggo:"dur1" default:"2s"`
	Dur2 func() time.Duration `cfggo:"dur2" default:"3s"`
	Dur3 func() time.Duration `cfggo:"dur3" default:"4s"`
	Dur4 func() time.Duration `cfggo:"dur4" default:"5s"`

	Slice0 func() []string       `cfggo:"slice0" default:"a,b,c"`
	Slice1 func() []int          `cfggo:"slice1" default:"1,2,3"`
	Slice2 func() []string       `cfggo:"slice2" default:"x,y,z"`
	Map0   func() map[string]int `cfggo:"map0"   default:"{\"a\":1}"`
	Map1   func() map[string]int `cfggo:"map1"   default:"{\"b\":2}"`
}

// --- In-memory ConfigHandler ------------------------------------------------

// memHandler is a sources.ConfigHandler backed by an in-memory JSON blob. It
// lets reload/load benchmarks measure cfggo's own work without paying for disk
// I/O on every iteration.
type memHandler struct {
	data   json.RawMessage
	defalt bool
}

func newMemHandler(data string) *memHandler {
	return &memHandler{data: json.RawMessage(data)}
}

func (h *memHandler) IsDefault() bool                      { return h.defalt }
func (h *memHandler) LoadConfig() (json.RawMessage, error) { return h.data, nil }
func (h *memHandler) SaveConfig(b json.RawMessage) error   { h.data = b; return nil }

// mediumJSON is a JSON document whose keys line up with mediumConfig, used by
// the file/handler-backed Init and reload benchmarks.
const mediumJSON = `{
  "app_name": "from-file",
  "port": 9090,
  "debug": true,
  "timeout": "45s",
  "max_conns": 250,
  "rate": 3.5,
  "ratio": 0.25,
  "log_level": "debug",
  "tags": ["x","y","z"],
  "ports": [10,20,30],
  "labels": {"env":"prod"},
  "limits": {"cpu":4,"mem":8},
  "retries": 5,
  "verbose": false,
  "region": "eu-west-1",
  "weight": 4.0
}`

// mustInit initialises cfg and fails the benchmark if Init errors. Options
// default to skipping the environment so unrelated process env state can never
// perturb a measurement; pass explicit options to opt back in.
func mustInit(b *testing.B, cfg interface{}, opts ...cfggo.Option) {
	b.Helper()
	if len(opts) == 0 {
		opts = []cfggo.Option{cfggo.WithSkipEnvironment()}
	}
	if err := cfggo.Init(cfg, opts...); err != nil {
		b.Fatalf("Init: %v", err)
	}
}

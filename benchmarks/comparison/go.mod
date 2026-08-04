// This is a SEPARATE Go module on purpose.
//
// The comparison benchmarks import competitor configuration libraries (Viper,
// envconfig, koanf) to measure cfggo against them. Keeping them in their own
// module means those third-party dependencies never enter cfggo's own go.mod /
// go.sum or its `go list -deps ./...` graph, so the root module stays
// dependency-free and the TestNoThirdPartyConfigImports guard keeps passing.
//
// Run with:
//
//	cd benchmarks/comparison && go test -bench=. -benchmem
module github.com/iqhive/cfggo/benchmarks/comparison

go 1.23.0

require (
	github.com/iqhive/cfggo v0.0.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/knadh/koanf/parsers/json v1.0.0
	github.com/knadh/koanf/providers/rawbytes v1.0.0
	github.com/knadh/koanf/v2 v2.3.5
	github.com/spf13/viper v1.21.0
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)

replace github.com/iqhive/cfggo => ../../

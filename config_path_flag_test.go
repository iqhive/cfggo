package cfggo

import (
	"flag"
	"os"
	"testing"
)

type configPathConfig struct {
	Structure
	Port func() int `cfggo:"port" default:"1"`
}

func TestFileConfigParamNameBeforeWithFlagSet(t *testing.T) {
	file := writeConfigFile(t, `{"port": 9}`)
	setProcessArgs(t, "--config="+file)
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithFileConfigParamName("config"), WithFlagSet(fs), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		t.Fatalf("host Parse: %v (the --config flag must be registered on the final flag set)", err)
	}
	if got := cfg.Port(); got != 9 {
		t.Fatalf("Port() = %d, want 9 from the bootstrapped file", got)
	}
}

func TestFileConfigParamNameAcceptsSingleDashForms(t *testing.T) {
	file := writeConfigFile(t, `{"port": 7}`)
	for _, args := range [][]string{
		{"-config", file},
		{"-config=" + file},
		{"--config", file},
		{"--config=" + file},
	} {
		t.Run(args[0], func(t *testing.T) {
			setProcessArgs(t, args...)
			cfg := &configPathConfig{}
			if err := cfg.Init(cfg, WithFileConfigParamName("config"), WithoutEnv()); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if got := cfg.Port(); got != 7 {
				t.Fatalf("Port() = %d, want 7", got)
			}
		})
	}
}

func TestFileConfigParamNameWithoutFlags(t *testing.T) {
	file := writeConfigFile(t, `{"port": 5}`)
	setProcessArgs(t, "--config="+file)
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithFileConfigParamName("config"), WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 5 {
		t.Fatalf("Port() = %d, want 5", got)
	}
}

func TestIgnoreUnknownFlagsDropsNegativeNumberValue(t *testing.T) {
	setProcessArgs(t, "--unknown", "-1", "--port=5")
	cfg := &configPathConfig{}
	if err := cfg.Init(cfg, WithIgnoreUnknownVars(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := cfg.Port(); got != 5 {
		t.Fatalf("Port() = %d, want 5: the -1 value of the unknown flag stopped parsing", got)
	}
}

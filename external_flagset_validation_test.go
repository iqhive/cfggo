package cfggo

import (
	"errors"
	"flag"
	"io"
	"os"
	"testing"
)

type tokenConfig struct {
	Structure
	Token func() string `cfggo:"token"`
	Port  func() int    `cfggo:"port" default:"8080"`
}

func TestExternalFlagSetDefersValidationForFlagsOnCommandLine(t *testing.T) {
	setProcessArgs(t, "--token=abc")
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	cfg := &tokenConfig{}
	if err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs), WithValidation("token", Required())); err != nil {
		t.Fatalf("Init = %v, want success: --token is on the command line and the host has not parsed yet", err)
	}
	if err := cfg.Validate(); !errors.Is(err, ErrValidation) {
		t.Fatalf("Validate before Parse = %v, want the still-empty token reported", err)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate after Parse: %v", err)
	}
	if got := cfg.Token(); got != "abc" {
		t.Fatalf("Token() = %q", got)
	}
}

func TestExternalFlagSetStillFailsInitWhenNoFlagIsPending(t *testing.T) {
	setProcessArgs(t, "--port=9090")
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	cfg := &tokenConfig{}
	err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs), WithValidation("token", Required()))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Init = %v, want a validation error: nothing on the command line can supply token", err)
	}
}

func TestExternalFlagSetRejectsInvalidValueDuringParse(t *testing.T) {
	setProcessArgs(t, "--port=70000")
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := &tokenConfig{}
	if err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs), WithValidation("port", Range(1, 65535))); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := fs.Parse(os.Args[1:]); err == nil {
		t.Fatal("Parse accepted --port=70000, want the flag's validator to reject it")
	}
	if got := cfg.Port(); got != 8080 {
		t.Fatalf("Port() = %d after a rejected flag, want the default 8080", got)
	}
}

func TestExternalFlagSetAlreadyParsedValidatesAtInit(t *testing.T) {
	setProcessArgs(t, "--token=abc")
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	cfg := &tokenConfig{}
	err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs), WithValidation("token", Required()))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Init = %v, want a validation error: the host has already parsed, so no value is pending", err)
	}
}

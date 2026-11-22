package cfggo

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/iqhive/cfggo/cfglogger"
)

// secretFlagConfig has one secret-tagged key and one ordinary key so tests can
// check that redaction is targeted at secrets only
type secretFlagConfig struct {
	Structure
	APIKey func() string `cfggo:"api.key" secret:"true"`
	Port   func() int    `cfggo:"port" default:"8080"`
}

const rawFlagSecret = "hunter2-RAW-SECRET-VALUE"

// setProcessArgs replaces os.Args for the duration of the test
func setProcessArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = append([]string{"test"}, args...)
}

func TestSecretFlagValueRedactedFromPrivateFlagSetParse(t *testing.T) {
	setProcessArgs(t, "--api.key="+rawFlagSecret)
	var logs, flagOutput bytes.Buffer
	cfg := &secretFlagConfig{}
	// Create the private flag set up front so its output can be captured
	cfg.GetFlagSet().SetOutput(&flagOutput)

	err := cfg.Init(cfg, WithoutEnv(),
		WithLogger(cfglogger.NewDefaultLoggerWithWriter(&logs)),
		WithValidation("api.key", MinLength(64)))
	if err == nil {
		t.Fatal("Init succeeded, want a validation failure for the short secret")
	}
	for name, text := range map[string]string{
		"Init error":      err.Error(),
		"log output":      logs.String(),
		"flag set output": flagOutput.String(),
	} {
		if strings.Contains(text, rawFlagSecret) {
			t.Errorf("%s leaks the raw secret: %s", name, text)
		}
	}
	if !strings.Contains(err.Error(), `"`+maskedValue+`"`) {
		t.Errorf("Init error = %q, want the flag package's message with the value masked", err)
	}
	if !strings.Contains(flagOutput.String(), `"`+maskedValue+`"`) {
		t.Errorf("flag set output = %q, want the masked message", flagOutput.String())
	}
}

func TestNonSecretFlagValueStaysInParseError(t *testing.T) {
	setProcessArgs(t, "--port=notanumber")
	cfg := &secretFlagConfig{}
	cfg.GetFlagSet().SetOutput(io.Discard)
	err := cfg.Init(cfg, WithoutEnv())
	if err == nil {
		t.Fatal("Init succeeded, want a conversion failure")
	}
	if !strings.Contains(err.Error(), `"notanumber"`) {
		t.Fatalf("Init error = %q, want the rejected value of a non-secret flag to be reported", err)
	}
}

func TestSecretFlagValueRedactedFromExternalFlagSetOutput(t *testing.T) {
	fs := flag.NewFlagSet("external", flag.ContinueOnError)
	var out bytes.Buffer
	fs.SetOutput(&out)
	cfg := &secretFlagConfig{}
	if err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg.RegisterValidator("api.key", MinLength(64))

	err := fs.Parse([]string{"--api.key=" + rawFlagSecret})
	if err == nil {
		t.Fatal("Parse succeeded, want a validation failure")
	}
	if strings.Contains(out.String(), rawFlagSecret) {
		t.Errorf("flag set output leaks the raw secret: %s", out.String())
	}
	if !strings.Contains(out.String(), `"`+maskedValue+`"`) {
		t.Errorf("flag set output = %q, want the masked message", out.String())
	}

	// The error the host receives from Parse is built by the flag package and
	// still carries the value; RedactFlagError scrubs it
	redacted := cfg.RedactFlagError(err)
	if strings.Contains(redacted.Error(), rawFlagSecret) {
		t.Errorf("RedactFlagError left the raw secret in place: %s", redacted)
	}
	if !strings.Contains(redacted.Error(), `"`+maskedValue+`"`) {
		t.Errorf("RedactFlagError = %q, want the masked message", redacted)
	}
	if unwrapped := errors.Unwrap(redacted); unwrapped != nil && strings.Contains(unwrapped.Error(), rawFlagSecret) {
		t.Errorf("redacted error still unwraps to the raw value")
	}

	other := errors.New("unrelated failure")
	if got := cfg.RedactFlagError(other); got != other {
		t.Errorf("RedactFlagError(unrelated) = %v, want the same error back", got)
	}
	if got := cfg.RedactFlagError(nil); got != nil {
		t.Errorf("RedactFlagError(nil) = %v, want nil", got)
	}
}

func TestRedactFlagTextOnlyTouchesOwnSecretFlags(t *testing.T) {
	cfg := &secretFlagConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	in := `invalid value "s3cret" for flag -api.key: too short` + "\n" +
		`invalid value "abc" for flag -port: not a number` + "\n" +
		`invalid boolean value "s3cret" for -api.key: no` + "\n" +
		`invalid value "s3cret" for flag -other.key: unknown`
	want := `invalid value "****" for flag -api.key: too short` + "\n" +
		`invalid value "abc" for flag -port: not a number` + "\n" +
		`invalid boolean value "****" for -api.key: no` + "\n" +
		`invalid value "s3cret" for flag -other.key: unknown`
	if got := cfg.redactFlagText(in); got != want {
		t.Fatalf("redactFlagText:\n got: %q\nwant: %q", got, want)
	}
	escaped := `invalid value "a\"b\\c" for flag -api.key: bad`
	if got := cfg.redactFlagText(escaped); got != `invalid value "****" for flag -api.key: bad` {
		t.Fatalf("redactFlagText(escaped) = %q", got)
	}
}

func TestUnknownFlagReturnsErrorInsteadOfExiting(t *testing.T) {
	setProcessArgs(t, "--zzqxv=1")
	cfg := &secretFlagConfig{}
	cfg.GetFlagSet().SetOutput(io.Discard)
	err := cfg.Init(cfg, WithoutEnv())
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("Init error = %v, want ErrUnknownKey for an unknown flag with no close match", err)
	}
	if !strings.Contains(err.Error(), "-zzqxv") || strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("Init error = %q, want the flag named without a suggestion", err)
	}
}

func TestUnknownFlagWithCloseMatchReturnsSuggestion(t *testing.T) {
	setProcessArgs(t, "--prot=1")
	cfg := &secretFlagConfig{}
	cfg.GetFlagSet().SetOutput(io.Discard)
	err := cfg.Init(cfg, WithoutEnv())
	if !errors.Is(err, ErrUnknownKey) || !strings.Contains(err.Error(), "did you mean -port?") {
		t.Fatalf("Init error = %v, want ErrUnknownKey with a -port suggestion", err)
	}
}

func TestBadFlagValueReturnsErrorInsteadOfExiting(t *testing.T) {
	setProcessArgs(t, "--port=abc")
	cfg := &secretFlagConfig{}
	cfg.GetFlagSet().SetOutput(io.Discard)
	err := cfg.Init(cfg, WithoutEnv())
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("Init error = %v, want a parse error naming the port flag", err)
	}
}

func TestHelpFlagReturnsErrHelpWithUsage(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"--zzqxv=1", "-h"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			setProcessArgs(t, args...)
			var usage bytes.Buffer
			cfg := &secretFlagConfig{}
			cfg.GetFlagSet().SetOutput(&usage)
			options := []Option{WithoutEnv()}
			if len(args) > 1 {
				options = append(options, WithIgnoreUnknownVars())
			}
			err := cfg.Init(cfg, options...)
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("Init error = %v, want flag.ErrHelp", err)
			}
			if !strings.Contains(usage.String(), "-port") {
				t.Fatalf("usage output = %q, want the known flags listed", usage.String())
			}
		})
	}
}

// specialNameSecretConfig holds secret keys whose flag names contain characters
// the standard flag package permits but which a character-class name match
// truncates at: a colon and a space. Redaction must recognise these names
// exactly rather than parsing the name out of the error text.
type specialNameSecretConfig struct {
	Structure
	ColonKey func() int `cfggo:"pin:code" secret:"true"`
	SpaceKey func() int `cfggo:"my pass" secret:"true"`
	PlainKey func() int `cfggo:"password" secret:"true"`
	Ordinary func() int `cfggo:"port" default:"8080"`
}

// TestSecretFlagValueRedactedForSpecialCharacterKeyNames drives a real parse
// failure for each secret key shape and asserts the rejected value never
// reaches the flag set output or the error a host passes through
// RedactFlagError. Before the fix, a secret whose flag name contained ':' or
// whitespace was not recognised as a secret and its raw value leaked.
func TestSecretFlagValueRedactedForSpecialCharacterKeyNames(t *testing.T) {
	for _, key := range []string{"password", "pin:code", "my pass"} {
		t.Run(key, func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			var out bytes.Buffer
			fs.SetOutput(&out)
			cfg := &specialNameSecretConfig{}
			if err := cfg.Init(cfg, WithoutEnv(), WithFlagSet(fs)); err != nil {
				t.Fatalf("Init: %v", err)
			}
			parseErr := fs.Parse([]string{"--" + key + "=" + rawFlagSecret})
			if parseErr == nil {
				t.Fatalf("Parse succeeded, want a conversion failure for %q", key)
			}
			if strings.Contains(out.String(), rawFlagSecret) {
				t.Errorf("flag set output leaks the raw secret for %q: %s", key, out.String())
			}
			redacted := cfg.RedactFlagError(parseErr)
			if strings.Contains(redacted.Error(), rawFlagSecret) {
				t.Errorf("RedactFlagError leaks the raw secret for %q: %s", key, redacted)
			}
			if !strings.Contains(redacted.Error(), `"`+maskedValue+`"`) {
				t.Errorf("RedactFlagError = %q, want the rejected value masked", redacted)
			}
		})
	}
}

// TestRedactFlagTextMatchesSecretFlagNamesExactly pins redactFlagText directly:
// each secret flag name (however exotic) is redacted, while unknown and
// non-secret flags are left untouched.
func TestRedactFlagTextMatchesSecretFlagNamesExactly(t *testing.T) {
	cfg := &specialNameSecretConfig{}
	if err := cfg.Init(cfg, WithoutFlags(), WithoutEnv()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cases := []struct{ in, want string }{
		{`invalid value "v" for flag -pin:code: bad`, `invalid value "****" for flag -pin:code: bad`},
		{`invalid value "v" for flag -my pass: bad`, `invalid value "****" for flag -my pass: bad`},
		{`invalid value "v" for flag -password: bad`, `invalid value "****" for flag -password: bad`},
		{`invalid boolean value "v" for -pin:code: bad`, `invalid boolean value "****" for -pin:code: bad`},
		{`invalid value "v" for flag -port: bad`, `invalid value "v" for flag -port: bad`},
		{`invalid value "v" for flag -notasecret: bad`, `invalid value "v" for flag -notasecret: bad`},
	}
	for _, tc := range cases {
		if got := cfg.redactFlagText(tc.in); got != tc.want {
			t.Errorf("redactFlagText(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
		}
	}
}

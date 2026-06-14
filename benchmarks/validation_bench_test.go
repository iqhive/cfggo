package benchmarks

import (
	"fmt"
	"testing"

	"github.com/iqhive/cfggo"
)

// Validation has two costs: running cfg.Validate()/ValidateKey() (which locks,
// walks config, and dispatches to registered validators) and the validators
// themselves. Both are measured here.

// BenchmarkValidate measures a full Validate() pass with a validator attached to
// every key, across a range of struct sizes.
func BenchmarkValidate(b *testing.B) {
	b.Run("medium", func(b *testing.B) {
		cfg := &mediumConfig{}
		mustInit(b, cfg)
		for _, k := range []string{"port", "max_conns", "retries"} {
			cfg.AddValidator(k, cfggo.Range(0, 1e9))
		}
		cfg.AddValidator("log_level", cfggo.OneOf("debug", "info", "warn", "error"))
		cfg.AddValidator("app_name", cfggo.Required())

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := cfg.Validate(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("large", func(b *testing.B) {
		cfg := &largeConfig{}
		mustInit(b, cfg)
		for i := 0; i < 10; i++ {
			cfg.AddValidator(fmt.Sprintf("int%d", i), cfggo.Range(-1, 1e9))
			cfg.AddValidator(fmt.Sprintf("str%d", i), cfggo.Required())
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := cfg.Validate(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkValidateNoValidators measures the fixed overhead of Validate() when
// nothing is registered (the common steady-state case).
func BenchmarkValidateNoValidators(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateKey(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)
	cfg.AddValidator("port", cfggo.Range(1, 65535))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.ValidateKey("port"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuiltinValidators measures each built-in validator in isolation,
// invoking the validator function directly with a passing value.
func BenchmarkBuiltinValidators(b *testing.B) {
	cases := []struct {
		name      string
		validator func(interface{}) error
		value     interface{}
	}{
		{"Range", cfggo.Range(0, 100), 42},
		{"OneOf", cfggo.OneOf("debug", "info", "warn", "error"), "info"},
		{"Required", cfggo.Required(), "non-empty"},
		{"MinLength", cfggo.MinLength(3), "hello"},
		{"MaxLength", cfggo.MaxLength(20), "hello"},
		{"Email", cfggo.Email(), "user@example.com"},
		{"URL", cfggo.URL(), "https://example.com/path"},
		{"Regex", cfggo.Regex(`^[A-Za-z0-9]{8}$`), "abcd1234"},
		{"All", cfggo.All(cfggo.Required(), cfggo.MinLength(3), cfggo.MaxLength(20)), "username"},
		{"Any", cfggo.Any(cfggo.MinLength(100), cfggo.Required()), "ok"},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = c.validator(c.value)
			}
		})
	}
}

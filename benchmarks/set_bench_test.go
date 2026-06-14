package benchmarks

import (
	"testing"
	"time"

	"github.com/iqhive/cfggo"
)

// Set converts the incoming value to the field's declared type, stores it,
// records provenance, and (when listeners exist) fires callbacks. These
// benchmarks measure the write + conversion path per value type. They use the
// same-typed value so they measure the fast assignment path; the *FromString
// variants force the string-coercion path that flags and env vars exercise.

func BenchmarkSetInt(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("port", 9000); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("app_name", "value"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetBool(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("debug", true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetFloat(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("rate", 9.5); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetDuration(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("timeout", 5*time.Second); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetSlice(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)
	val := []string{"p", "q", "r"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("tags", val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSetMap(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)
	val := map[string]string{"a": "1", "b": "2"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("labels", val); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetIntFromString forces the string->int coercion path.
func BenchmarkSetIntFromString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("port", "9000"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetDurationFromString forces the string->time.Duration coercion path.
func BenchmarkSetDurationFromString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("timeout", "1h30m"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetSliceFromString forces the comma-separated string->[]string path.
func BenchmarkSetSliceFromString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("tags", "p,q,r"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetMapFromString forces the JSON-object string->map path.
func BenchmarkSetMapFromString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("labels", `{"a":"1","b":"2"}`); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSetWithListener measures the added cost of dispatching to a single
// registered OnChange callback on every Set.
func BenchmarkSetWithListener(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)
	var fired int
	cancel := cfg.OnChange(func(changes []cfggo.Change) { fired += len(changes) })
	defer cancel()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cfg.Set("port", i); err != nil {
			b.Fatal(err)
		}
	}
	_ = fired
}

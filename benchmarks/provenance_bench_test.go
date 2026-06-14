package benchmarks

import (
	"testing"
)

// Provenance and dump helpers are the debugging surface (Source/Sources/
// Explain/String/GetJSONBytes). They walk and format the whole config, so they
// are measured against the medium and large structs.

func BenchmarkSource(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cfg.Source("port")
	}
}

func BenchmarkSources(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Sources()
	}
}

func BenchmarkExplainMedium(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.Explain()
	}
	_ = sink
}

func BenchmarkExplainLarge(b *testing.B) {
	cfg := &largeConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.Explain()
	}
	_ = sink
}

func BenchmarkString(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink string
	for i := 0; i < b.N; i++ {
		sink = cfg.String()
	}
	_ = sink
}

func BenchmarkGetJSONBytes(b *testing.B) {
	cfg := &mediumConfig{}
	mustInit(b, cfg)

	b.ReportAllocs()
	b.ResetTimer()
	var sink []byte
	for i := 0; i < b.N; i++ {
		sink, _ = cfg.GetJSONBytes()
	}
	_ = sink
}

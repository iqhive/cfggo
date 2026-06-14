// Package benchmarks contains the performance benchmark suite for cfggo.
//
// The benchmarks live in their own package (and directory) so they exercise
// cfggo strictly through its public API — the same surface real callers use —
// and so they never affect the import graph or test-time dependencies of the
// core library.
//
// # Running
//
// Run the whole suite with allocation stats:
//
//	go test -bench=. -benchmem ./benchmarks/
//
// Run a single group (for example just the accessor reads):
//
//	go test -bench=Accessor -benchmem ./benchmarks/
//
// Get more stable numbers by increasing the per-benchmark run time and
// repeating, which pairs well with golang.org/x/perf/cmd/benchstat:
//
//	go test -bench=. -benchmem -benchtime=2s -count=10 ./benchmarks/ | tee new.txt
//	benchstat new.txt
//
// # What is measured
//
//   - accessor_bench_test.go   typed accessor reads (the headline read path)
//   - init_bench_test.go       Init cost vs. struct size and config sources
//   - set_bench_test.go        Set + type-conversion cost per value type
//   - reload_bench_test.go     hot-reload cost, with and without listeners
//   - provenance_bench_test.go Source/Sources/Explain/String/GetJSONBytes
//   - validation_bench_test.go Validate/ValidateKey and the built-in validators
//   - concurrent_bench_test.go contended read, write, and mixed workloads
package benchmarks

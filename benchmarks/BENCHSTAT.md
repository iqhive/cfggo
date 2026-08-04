# cfggo.Group benchstat comparison

Run against `main` and `feature/group-swe` with `go test -bench=... -count=6 ./benchmarks`.

## Existing benchmarks (no regression)

Filtered to benchmarks present on both branches. All existing Init, accessor,
reload, set, validation, and concurrency benchmarks are unchanged.

```
                                   │   main.txt    │ feature.txt   │
                                   │    sec/op     │    sec/op     │ vs base │
geomean                                            356.5n          356.9n   +0.12%

InitSmall-8                                        3.850µ ± 2%     3.877µ ± 1%   ~
InitMedium-8                                       15.20µ ± 5%     15.18µ ± 2%   ~
InitLarge-8                                        27.85µ ± 2%     27.98µ ± 1%   ~
InitWithFile-8                                     33.27µ ± 2%     33.06µ ± 1%   ~
InitWithEnv-8                                      19.78µ ± 1%     19.64µ ± 1%   ~
AccessorInt-8                                      15.38n ± 2%     15.36n ± 9%    ~
AccessorString-8                                   15.52n ± 1%     15.32n ± 1%   ~
AccessorParallel-8                                 43.23n ± 5%     44.47n ± 2%   ~
ConcurrentReads-8                                  44.94n ± 4%     43.67n ± 2%   ~
```

Memory allocations are identical for all shared benchmarks.

## New Group benchmarks

| Benchmark                    | time/op  | B/op   | allocs/op |
|------------------------------|----------|--------|-----------|
| StandaloneRead               | 30.41n   | 0      | 0         |
| GroupMemberRead              | 30.39n   | 0      | 0         |
| GroupInit2Members            | 10.43µ   | 8.46Ki | 159       |
| GroupInit4Members            | 21.18µ   | 16.66Ki| 308       |
| GroupInit2MembersWithFile    | 21.57µ   | 13.05Ki| 229       |

Group member reads are within measurement noise of standalone reads.

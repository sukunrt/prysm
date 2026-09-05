# Benchmark host and tool environment

Inventory captured on 2026-09-08 before compilation or timing:

| Item | Value |
| --- | --- |
| CPU | AMD Ryzen 7 7840U with Radeon 780M Graphics |
| Topology | 8 cores / 16 threads, one NUMA node |
| Frequency range | 419.175 MHz–5,134.889 MHz; boost enabled |
| L3 cache | 16 MiB |
| Memory | 60 GiB total; 46 GiB available at inventory time |
| Swap | 49 GiB total; essentially unused |
| Workspace filesystem | NVMe, 881 GiB total, 60 GiB available, 93% used |
| Go | `go1.26.5 linux/amd64` |
| Repository Bazel version | `7.4.1` from `.bazelversion` |
| Bazel launcher | `/home/sukun/go/bin/bazelisk` (plain `bazel` is not on `PATH`) |
| Go build cache | `/home/sukun/.cache/go-build`, 17 GiB |
| Go module cache | `/home/sukun/go/pkg/mod`, 18 GiB |

The benchmark runner must record `GOMAXPROCS`, CPU governor/frequency state,
command, iteration count, elapsed time, and allocations with each result. Run
the timing processes serially; the inventory above does not claim that the host
was idle.

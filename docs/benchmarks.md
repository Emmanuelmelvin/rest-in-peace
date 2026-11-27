# RIP BASIC Benchmarks

Reproducible benchmark measurements for **REST in Peace (RIP BASIC)** against Requirements 14.1 and 14.2.

## Spec targets

| Metric | Spec requirement | Target |
|---|---|---|
| Throughput (`GET /hello`) | Requirement 14.1 | >= 10,000 req/s on 1 vCPU (measured with `oha` or `wrk`) |
| Idle memory (RSS) | Requirement 14.2 | <= 20 MB |
| Binary size (`rip build`) | Requirement 14.2 | <= 30 MB |

## Test machine

- **CPU:** Intel Core i5-8265U @ 1.60 GHz (4 physical cores, 8 logical processors)
- **OS:** Microsoft Windows 11 Pro (`10.0.26300`, 64-bit, `windows/amd64`)
- **Go:** `go1.27.4`
- **Load generator:** built-in Go HTTP/1.1 keep-alive load generator (`bench/loadgen.go`), `-c 64 -d 10s -warmup 2s` (`oha` and `wrk` were not installed on this machine; `bench` runs them automatically when present in `PATH`)
- **Targets:**
  - `examples/hello.bas` compiled with `go run ./cmd/rip build` (`CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`)
  - Equivalent plain `net/http` Go server (`bench/baseline/main.go`) built with the same release flags

## Results

### 1. Server pinned to `GOMAXPROCS=1` (1-vCPU approximation)

| Metric | RIP (`examples/hello.bas`) | Go `net/http` baseline | Spec target | Status (RIP) |
|---|---:|---:|---|---|
| Binary size | 10.78 MiB (11.31 MB) | 6.50 MiB (6.82 MB) | <= 30 MB | **PASS** |
| Startup to first 200 | 243.6 ms | 3.57 s* | — | — |
| Idle RSS (after startup) | 8.93 MiB (9.37 MB) | 8.02 MiB (8.40 MB) | <= 20 MB | **PASS** |
| Idle RSS (2 s post-load) | 55.41 MiB (58.10 MB) | 50.45 MiB (52.90 MB) | <= 20 MB | **MISS** |
| Throughput (`GET /hello`) | 6,668.7 req/s | 6,414.7 req/s | >= 10,000 req/s | **MISS** |
| Latency p50 / p95 / p99 | 7.93 ms / 20.01 ms / 45.95 ms | 7.13 ms / 24.61 ms / 52.45 ms | — | — |
| Errors / non-200 | 0 / 0 (66,740 reqs) | 0 / 0 (64,183 reqs) | — | — |

### 2. Server with default `GOMAXPROCS` (8 logical CPUs)

| Metric | RIP (`examples/hello.bas`) | Go `net/http` baseline | Spec target | Status (RIP) |
|---|---:|---:|---|---|
| Binary size | 10.78 MiB (11.31 MB) | 6.50 MiB (6.82 MB) | <= 30 MB | **PASS** |
| Startup to first 200 | 521.9 ms | 5.02 s* | — | — |
| Idle RSS (after startup) | 9.45 MiB (9.91 MB) | 8.41 MiB (8.81 MB) | <= 20 MB | **PASS** |
| Idle RSS (2 s post-load) | 19.73 MiB (20.69 MB) | 53.90 MiB (56.52 MB) | <= 20 MB | **PASS** (MiB) / **MISS** (decimal MB) |
| Throughput (`GET /hello`) | 6,724.3 req/s | 9,431.7 req/s | >= 10,000 req/s | **MISS** |
| Latency p50 / p95 / p99 | 4.16 ms / 29.75 ms / 96.19 ms | 3.42 ms / 19.07 ms / 57.82 ms | — | — |
| Errors / non-200 | 0 / 0 (67,349 reqs) | 0 / 0 (94,345 reqs) | — | — |

\* Cold-launch startup-to-first-200 times on Windows reflect Windows Defender scanning freshly compiled unsigned `.exe` binaries in a temp directory on first execution, not Go/RIP initialization time.

## Honest assessment & caveats

1. **Binary size (PASS):** `examples/hello.bas` compiles to **10.78 MiB (11.31 MB)**, well under the 30 MB cap and only ~4.3 MiB larger than the plain `net/http` baseline (6.50 MiB).
2. **Idle RSS after startup (PASS):** **8.93–9.45 MiB** (9.37–9.91 MB), comfortably under the 20 MB cap and within ~1 MiB of plain `net/http`.
3. **Idle RSS immediately after load (MISS at `GOMAXPROCS=1`):** 2 seconds after a 64-connection burst, RSS was **55.41 MiB** for RIP and **50.45 MiB** for the plain `net/http` baseline. Go retains recently allocated heap spans and returns them to the OS lazily via the background scavenger; setting `GOMEMLIMIT` or waiting longer allows RSS to drop back down.
4. **Throughput (MISS on this machine):** Neither RIP (**6,668.7 req/s**) nor the plain `net/http` Go baseline (**6,414.7 req/s**) reached 10,000 req/s at `GOMAXPROCS=1` on this Windows 11 laptop. At `GOMAXPROCS=1`, RIP matched (and slightly exceeded) plain `net/http`, showing that the ~6.7k req/s ceiling here is dominated by Windows loopback TCP overhead, a low-power 1.6 GHz mobile CPU, and the Go load generator sharing the same host. At `GOMAXPROCS=8`, plain `net/http` scaled to 9,431.7 req/s while RIP stayed at 6,724.3 req/s — worth profiling in `rt` (per-request allocations, `crypto/rand` request IDs, `rt.Map` JSON encoding) when targeting Linux 1-vCPU production numbers.
5. **Windows 1-vCPU pinning limits:** `bench` sets `GOMAXPROCS=1` on the server process to approximate a 1-vCPU deployment portably across Windows, Linux, and macOS. On Windows, `GOMAXPROCS=1` restricts Go user-space execution to one logical thread, but does not hard-pin OS threads to a single core (unlike Linux `taskset -c 0` or a 1-vCPU container/VM) and shares loopback + CPU cache with the load generator. Re-running `bench` with `oha` or `wrk` on a Linux 1-vCPU host is recommended for final Requirement 14.1 verification.
6. **Windows RSS definition:** On Windows, `bench` queries `WorkingSetSize` via `K32GetProcessMemoryInfo` (the closest OS equivalent to Linux `VmRSS`). On Linux it reads `VmRSS` from `/proc/<pid>/status`; on macOS it falls back to `ps -o rss=`.

## How to reproduce

From the repository root:

```sh
cd bench
go run . -c 64 -d 10s -warmup 2s -gomaxprocs 1 -compare
```

For default `GOMAXPROCS` (all logical CPUs):

```sh
cd bench
go run . -c 64 -d 10s -warmup 2s -gomaxprocs 0 -compare
```

To emit machine-readable JSON:

```sh
cd bench
go run . -c 64 -d 10s -warmup 2s -gomaxprocs 1 -compare -json
```

If `oha` or `wrk` is installed in your `PATH`, `bench` automatically detects and runs it against the same server instance after the Go load generator (disable with `-external=false`). You can also run `oha` or `wrk` directly against a built binary:

```sh
go run ./cmd/rip build -o ./dist/hello examples/hello.bas
GOMAXPROCS=1 PORT=8080 ./dist/hello
# in another terminal:
oha -c 64 -z 10s http://127.0.0.1:8080/hello
# or:
wrk -t 4 -c 64 -d 10s http://127.0.0.1:8080/hello
```

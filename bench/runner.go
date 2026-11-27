package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Spec targets from Requirements 14.1 and 14.2.
const (
	TargetReqPerSec    = 10000.0
	TargetIdleRSSBytes = 20 * 1024 * 1024 // 20 MiB (we also report decimal MB)
	TargetBinaryBytes  = 30 * 1024 * 1024 // 30 MiB (we also report decimal MB)
)

// Config controls a benchmark run.
type Config struct {
	RepoRoot    string
	BenchDir    string
	SourceFile  string
	Path        string
	Concurrency int
	Duration    time.Duration
	Warmup      time.Duration
	GoMaxProcs  int
	Compare     bool
	UseExternal bool
}

// TargetReport holds all measurements for one binary (RIP or baseline).
type TargetReport struct {
	Name                  string        `json:"name"`
	BuildDuration         time.Duration `json:"build_duration_ns"`
	BinarySizeBytes       int64         `json:"binary_size_bytes"`
	StartupToFirst200     time.Duration `json:"startup_to_first_200_ns"`
	IdleRSSStartupBytes   uint64        `json:"idle_rss_startup_bytes"`
	IdleRSSAfterLoadBytes uint64        `json:"idle_rss_after_load_bytes"`
	GoLoad                LoadResult    `json:"go_load"`
	ExternalLoad          *LoadResult   `json:"external_load,omitempty"`
}

// BenchmarkReport is the full JSON/text report for a benchmark invocation.
type BenchmarkReport struct {
	Timestamp    time.Time      `json:"timestamp"`
	GOOS         string         `json:"goos"`
	GOARCH       string         `json:"goarch"`
	GoVersion    string         `json:"go_version"`
	NumCPU       int            `json:"num_cpu"`
	GoMaxProcs   int            `json:"server_gomaxprocs"`
	SourceFile   string         `json:"source_file"`
	Path         string         `json:"path"`
	Concurrency  int            `json:"concurrency"`
	Duration     time.Duration  `json:"duration_ns"`
	Warmup       time.Duration  `json:"warmup_ns"`
	ExternalTool string         `json:"external_tool"`
	Targets      []TargetReport `json:"targets"`
}

func findRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "rip", "main.go")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repo root (cmd/rip/main.go) from %s", start)
		}
		dir = parent
	}
}

func exeName(base string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(base), ".exe") {
		return base + ".exe"
	}
	return base
}

func buildRipBinary(ctx context.Context, repoRoot, sourceRel, outPath string) (time.Duration, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/rip", "build", "-o", outPath, sourceRel)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("rip build %s failed: %w\n%s", sourceRel, err, buf.String())
	}
	return time.Since(start), nil
}

func buildBaselineBinary(ctx context.Context, benchDir, outPath string) (time.Duration, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags=-s -w", "-o", outPath, "./baseline")
	cmd.Dir = benchDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("baseline build failed: %w\n%s", err, buf.String())
	}
	return time.Since(start), nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected listener address type")
	}
	return addr.Port, nil
}

func waitForFirst200(ctx context.Context, targetURL string, timeout time.Duration) (time.Duration, error) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	start := time.Now()
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		resp, err := client.Get(targetURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return time.Since(start), nil
			}
		}
		time.Sleep(1 * time.Millisecond)
	}
	return 0, fmt.Errorf("server did not return 200 on %s within %s", targetURL, timeout)
}

func benchmarkBinary(ctx context.Context, cfg Config, name, binPath string, buildDur time.Duration, extTool, extPath string) (TargetReport, error) {
	info, err := os.Stat(binPath)
	if err != nil {
		return TargetReport{}, fmt.Errorf("stat %s: %w", binPath, err)
	}

	port, err := freePort()
	if err != nil {
		return TargetReport{}, fmt.Errorf("allocate free port: %w", err)
	}
	targetURL := fmt.Sprintf("http://127.0.0.1:%d%s", port, cfg.Path)

	cmd := exec.Command(binPath)
	env := append(os.Environ(), fmt.Sprintf("PORT=%d", port))
	if cfg.GoMaxProcs > 0 {
		env = append(env, fmt.Sprintf("GOMAXPROCS=%d", cfg.GoMaxProcs))
	}
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return TargetReport{}, fmt.Errorf("start %s: %w", name, err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	waitDur, err := waitForFirst200(ctx, targetURL, 15*time.Second)
	if err != nil {
		return TargetReport{}, fmt.Errorf("%s startup failed: %w\nstderr: %s", name, err, stderr.String())
	}
	startupTotal := time.Since(start)
	// startupTotal includes Process.Start + waitForFirst200; guard against clock skew.
	if startupTotal < waitDur {
		startupTotal = waitDur
	}

	// Let the newly started server settle briefly before sampling idle RSS.
	time.Sleep(500 * time.Millisecond)
	rssStartup, err := processRSSBytes(cmd.Process.Pid)
	if err != nil {
		return TargetReport{}, fmt.Errorf("%s startup RSS: %w", name, err)
	}

	goLoad, err := runGoLoad(ctx, targetURL, cfg.Concurrency, cfg.Warmup, cfg.Duration)
	if err != nil {
		return TargetReport{}, fmt.Errorf("%s go-loadgen: %w", name, err)
	}

	var extRes *LoadResult
	if extTool != "" {
		res, err := runExternalLoad(ctx, extTool, extPath, targetURL, cfg.Concurrency, cfg.Duration)
		if err != nil {
			return TargetReport{}, fmt.Errorf("%s %s: %w", name, extTool, err)
		}
		extRes = &res
	}

	// Allow connections to close and runtime to settle before sampling post-load RSS.
	time.Sleep(2 * time.Second)
	rssAfterLoad, err := processRSSBytes(cmd.Process.Pid)
	if err != nil {
		return TargetReport{}, fmt.Errorf("%s post-load RSS: %w", name, err)
	}

	return TargetReport{
		Name:                  name,
		BuildDuration:         buildDur,
		BinarySizeBytes:       info.Size(),
		StartupToFirst200:     startupTotal,
		IdleRSSStartupBytes:   rssStartup,
		IdleRSSAfterLoadBytes: rssAfterLoad,
		GoLoad:                goLoad,
		ExternalLoad:          extRes,
	}, nil
}

func runBenchmark(ctx context.Context, cfg Config, logOut io.Writer) (BenchmarkReport, error) {
	tmpDir, err := os.MkdirTemp("", "rip-bench-*")
	if err != nil {
		return BenchmarkReport{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var extTool, extPath string
	if cfg.UseExternal {
		extTool, extPath = detectExternalLoadTool()
	}

	report := BenchmarkReport{
		Timestamp:    time.Now().UTC(),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		GoVersion:    runtime.Version(),
		NumCPU:       runtime.NumCPU(),
		GoMaxProcs:   cfg.GoMaxProcs,
		SourceFile:   cfg.SourceFile,
		Path:         cfg.Path,
		Concurrency:  cfg.Concurrency,
		Duration:     cfg.Duration,
		Warmup:       cfg.Warmup,
		ExternalTool: extTool,
	}

	ripOut := filepath.Join(tmpDir, exeName("hello-rip"))
	fmt.Fprintf(logOut, "Building %s with `go run ./cmd/rip build`...\n", cfg.SourceFile)
	ripBuildDur, err := buildRipBinary(ctx, cfg.RepoRoot, cfg.SourceFile, ripOut)
	if err != nil {
		return BenchmarkReport{}, err
	}
	fmt.Fprintf(logOut, "Benchmarking RIP binary (%s, GOMAXPROCS=%s, -c %d, -d %s)...\n",
		filepath.Base(ripOut), formatGoMaxProcs(cfg.GoMaxProcs), cfg.Concurrency, cfg.Duration)
	ripTarget, err := benchmarkBinary(ctx, cfg, "RIP ("+cfg.SourceFile+")", ripOut, ripBuildDur, extTool, extPath)
	if err != nil {
		return BenchmarkReport{}, err
	}
	report.Targets = append(report.Targets, ripTarget)

	if cfg.Compare {
		baseOut := filepath.Join(tmpDir, exeName("hello-nethttp"))
		fmt.Fprintf(logOut, "Building plain net/http baseline...\n")
		baseBuildDur, err := buildBaselineBinary(ctx, cfg.BenchDir, baseOut)
		if err != nil {
			return BenchmarkReport{}, err
		}
		fmt.Fprintf(logOut, "Benchmarking plain net/http baseline (%s, GOMAXPROCS=%s, -c %d, -d %s)...\n",
			filepath.Base(baseOut), formatGoMaxProcs(cfg.GoMaxProcs), cfg.Concurrency, cfg.Duration)
		baseTarget, err := benchmarkBinary(ctx, cfg, "Go net/http baseline", baseOut, baseBuildDur, extTool, extPath)
		if err != nil {
			return BenchmarkReport{}, err
		}
		report.Targets = append(report.Targets, baseTarget)
	}

	return report, nil
}

func formatGoMaxProcs(n int) string {
	if n <= 0 {
		return fmt.Sprintf("default (%d)", runtime.NumCPU())
	}
	return strconv.Itoa(n)
}

func bytesToMiB(b uint64) float64 {
	return float64(b) / (1024 * 1024)
}

func bytesToMB(b uint64) float64 {
	return float64(b) / 1e6
}

func printReport(w io.Writer, rep BenchmarkReport) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== RIP BASIC Benchmark (%s/%s, %s, %d logical CPUs) ===\n",
		rep.GOOS, rep.GOARCH, rep.GoVersion, rep.NumCPU)
	fmt.Fprintf(w, "Endpoint: GET %s | Server GOMAXPROCS=%s | Concurrency=%d | Warmup=%s | Duration=%s\n",
		rep.Path, formatGoMaxProcs(rep.GoMaxProcs), rep.Concurrency, rep.Warmup, rep.Duration)
	if rep.ExternalTool != "" {
		fmt.Fprintf(w, "External load tool: %s\n", rep.ExternalTool)
	} else {
		fmt.Fprintln(w, "External load tool: none found in PATH (oha/wrk not installed)")
	}
	fmt.Fprintln(w)

	for _, t := range rep.Targets {
		fmt.Fprintf(w, "--- %s ---\n", t.Name)
		fmt.Fprintf(w, "  Build time:           %s\n", t.BuildDuration.Round(time.Millisecond))
		fmt.Fprintf(w, "  Binary size:          %.2f MiB (%.2f MB, %d bytes)  [target <= 30 MB: %s]\n",
			bytesToMiB(uint64(t.BinarySizeBytes)), bytesToMB(uint64(t.BinarySizeBytes)), t.BinarySizeBytes,
			passFail(uint64(t.BinarySizeBytes) <= TargetBinaryBytes))
		fmt.Fprintf(w, "  Startup to first 200: %s\n", t.StartupToFirst200.Round(100*time.Microsecond))
		fmt.Fprintf(w, "  Idle RSS (startup):   %.2f MiB (%.2f MB)  [target <= 20 MB: %s]\n",
			bytesToMiB(t.IdleRSSStartupBytes), bytesToMB(t.IdleRSSStartupBytes),
			passFail(t.IdleRSSStartupBytes <= TargetIdleRSSBytes))
		fmt.Fprintf(w, "  Idle RSS (post-load): %.2f MiB (%.2f MB)  [target <= 20 MB: %s]\n",
			bytesToMiB(t.IdleRSSAfterLoadBytes), bytesToMB(t.IdleRSSAfterLoadBytes),
			passFail(t.IdleRSSAfterLoadBytes <= TargetIdleRSSBytes))
		printLoadResult(w, t.GoLoad)
		if t.ExternalLoad != nil {
			printLoadResult(w, *t.ExternalLoad)
		}
		fmt.Fprintln(w)
	}
}

func printLoadResult(w io.Writer, lr LoadResult) {
	fmt.Fprintf(w, "  Throughput (%s): %.1f req/s (%d reqs in %s, errors=%d, non200=%d)  [target >= 10,000 req/s: %s]\n",
		lr.Tool, lr.ReqPerSec, lr.Requests, lr.Duration.Round(time.Millisecond),
		lr.Errors, lr.Non200, passFail(lr.ReqPerSec >= TargetReqPerSec))
	if lr.LatencyP50 > 0 || lr.LatencyP99 > 0 {
		fmt.Fprintf(w, "    Latency: min=%s p50=%s p95=%s p99=%s max=%s\n",
			lr.LatencyMin.Round(time.Microsecond),
			lr.LatencyP50.Round(time.Microsecond),
			lr.LatencyP95.Round(time.Microsecond),
			lr.LatencyP99.Round(time.Microsecond),
			lr.LatencyMax.Round(time.Microsecond))
	}
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "MISS"
}

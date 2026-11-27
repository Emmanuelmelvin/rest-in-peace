package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	var (
		repoFlag    = flag.String("repo", "", "path to repository root (auto-detected if empty)")
		sourceFlag  = flag.String("source", "examples/hello.bas", "BASIC source file relative to repo root")
		pathFlag    = flag.String("path", "/hello", "HTTP path to request")
		concurrency = flag.Int("c", 64, "number of concurrent HTTP workers")
		duration    = flag.Duration("d", 10*time.Second, "load test duration")
		warmup      = flag.Duration("warmup", 2*time.Second, "warmup duration before measuring")
		gomaxprocs  = flag.Int("gomaxprocs", 1, "GOMAXPROCS for server process (1 approximates 1 vCPU; 0 = default)")
		compare     = flag.Bool("compare", true, "also benchmark equivalent plain net/http Go baseline")
		useExternal = flag.Bool("external", true, "also run oha or wrk if installed in PATH")
		jsonOutput  = flag.Bool("json", false, "output report as JSON")
	)
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getwd: %v\n", err)
		os.Exit(1)
	}

	repoRoot := *repoFlag
	if repoRoot == "" {
		repoRoot, err = findRepoRoot(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		repoRoot, err = filepath.Abs(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	benchDir := filepath.Join(repoRoot, "bench")
	cfg := Config{
		RepoRoot:    repoRoot,
		BenchDir:    benchDir,
		SourceFile:  *sourceFlag,
		Path:        *pathFlag,
		Concurrency: *concurrency,
		Duration:    *duration,
		Warmup:      *warmup,
		GoMaxProcs:  *gomaxprocs,
		Compare:     *compare,
		UseExternal: *useExternal,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logOut := os.Stderr
	if !*jsonOutput {
		logOut = os.Stdout
	}

	report, err := runBenchmark(ctx, cfg, logOut)
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchmark failed: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printReport(os.Stdout, report)
}

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessRSSBytesSelf(t *testing.T) {
	rss, err := processRSSBytes(os.Getpid())
	if err != nil {
		t.Fatalf("processRSSBytes(self) error: %v", err)
	}
	if rss == 0 {
		t.Fatalf("expected non-zero RSS for current process")
	}
	const tenGiB = 10 * 1024 * 1024 * 1024
	if rss > tenGiB {
		t.Fatalf("unexpectedly huge RSS: %d bytes", rss)
	}
}

func TestPercentile(t *testing.T) {
	samples := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		6 * time.Millisecond,
		7 * time.Millisecond,
		8 * time.Millisecond,
		9 * time.Millisecond,
		10 * time.Millisecond,
	}
	tests := []struct {
		p    float64
		want time.Duration
	}{
		{0.0, 1 * time.Millisecond},
		{0.50, 5 * time.Millisecond},
		{0.95, 10 * time.Millisecond},
		{0.99, 10 * time.Millisecond},
		{1.0, 10 * time.Millisecond},
	}
	for _, tc := range tests {
		got := percentile(samples, tc.p)
		if got != tc.want {
			t.Errorf("percentile(p=%.2f) = %v, want %v", tc.p, got, tc.want)
		}
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile(nil) = %v, want 0", got)
	}
}

func TestParseOhaJSON(t *testing.T) {
	raw := []byte(`{
		"summary": {
			"total": 10.002,
			"fastest": 0.00012,
			"slowest": 0.015,
			"requestsPerSec": 24500.5
		},
		"latencyPercentiles": {
			"p50": 0.0012,
			"p95": 0.0045,
			"p99": 0.0081
		},
		"statusCodeDistribution": {
			"200": 245000,
			"500": 5
		}
	}`)
	res, err := parseOhaJSON(raw, 64, 10*time.Second)
	if err != nil {
		t.Fatalf("parseOhaJSON error: %v", err)
	}
	if res.Tool != "oha" {
		t.Errorf("Tool = %q, want oha", res.Tool)
	}
	if res.ReqPerSec != 24500.5 {
		t.Errorf("ReqPerSec = %v, want 24500.5", res.ReqPerSec)
	}
	if res.Requests != 245005 {
		t.Errorf("Requests = %d, want 245005", res.Requests)
	}
	if res.Non200 != 5 {
		t.Errorf("Non200 = %d, want 5", res.Non200)
	}
	if res.LatencyP50 <= 0 || res.LatencyP99 <= res.LatencyP50 {
		t.Errorf("unexpected latency percentiles: p50=%v p99=%v", res.LatencyP50, res.LatencyP99)
	}
}

func TestParseWrkOutput(t *testing.T) {
	sample := `Running 10s test @ http://127.0.0.1:8080/hello
  4 threads and 64 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     2.10ms    1.05ms  18.20ms   89.12%
    Req/Sec     7.55k   620.00     9.10k    75.00%
  301240 requests in 10.01s, 42.10MB read
Requests/sec:  30093.91
Transfer/sec:      4.21MB`

	res, err := parseWrkOutput(sample, 64, 10*time.Second)
	if err != nil {
		t.Fatalf("parseWrkOutput error: %v", err)
	}
	if res.Tool != "wrk" {
		t.Errorf("Tool = %q, want wrk", res.Tool)
	}
	if res.ReqPerSec != 30093.91 {
		t.Errorf("ReqPerSec = %v, want 30093.91", res.ReqPerSec)
	}
	if res.Requests != 301240 {
		t.Errorf("Requests = %d, want 301240", res.Requests)
	}
}

func TestRunGoLoad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"HELLO WORLD"}`))
	}))
	defer srv.Close()

	res, err := runGoLoad(context.Background(), srv.URL, 4, 50*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("runGoLoad error: %v", err)
	}
	if res.Requests == 0 {
		t.Fatalf("expected > 0 requests")
	}
	if res.Errors != 0 || res.Non200 != 0 {
		t.Fatalf("expected 0 errors and 0 non-200, got errors=%d non200=%d", res.Errors, res.Non200)
	}
	if res.ReqPerSec <= 0 {
		t.Fatalf("expected positive ReqPerSec, got %v", res.ReqPerSec)
	}
	if res.LatencyMax < res.LatencyMin {
		t.Fatalf("max latency %v < min latency %v", res.LatencyMax, res.LatencyMin)
	}
}

func TestFindRepoRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		t.Fatalf("findRepoRoot(%s) error: %v", cwd, err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "rip", "main.go")); err != nil {
		t.Fatalf("expected cmd/rip/main.go under %s: %v", root, err)
	}
}

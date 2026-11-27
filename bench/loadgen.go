package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LoadResult summarizes one HTTP load-test run.
type LoadResult struct {
	Tool        string        `json:"tool"`
	Concurrency int           `json:"concurrency"`
	Duration    time.Duration `json:"duration_ns"`
	Requests    int64         `json:"requests"`
	Errors      int64         `json:"errors"`
	Non200      int64         `json:"non_200"`
	ReqPerSec   float64       `json:"req_per_sec"`
	LatencyMin  time.Duration `json:"latency_min_ns,omitempty"`
	LatencyP50  time.Duration `json:"latency_p50_ns,omitempty"`
	LatencyP95  time.Duration `json:"latency_p95_ns,omitempty"`
	LatencyP99  time.Duration `json:"latency_p99_ns,omitempty"`
	LatencyMax  time.Duration `json:"latency_max_ns,omitempty"`
	RawOutput   string        `json:"raw_output,omitempty"`
}

const maxSamplesPerWorker = 10000

// runGoLoad runs a built-in HTTP/1.1 keep-alive load test against targetURL.
func runGoLoad(ctx context.Context, targetURL string, concurrency int, warmup, duration time.Duration) (LoadResult, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	if duration <= 0 {
		return LoadResult{}, errors.New("duration must be positive")
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        concurrency * 2,
		MaxIdleConnsPerHost: concurrency * 2,
		MaxConnsPerHost:     concurrency,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	if warmup > 0 {
		warmCtx, cancel := context.WithTimeout(ctx, warmup)
		var wg sync.WaitGroup
		for range concurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for warmCtx.Err() == nil {
					req, err := http.NewRequestWithContext(warmCtx, http.MethodGet, targetURL, nil)
					if err != nil {
						return
					}
					resp, err := client.Do(req)
					if err != nil {
						continue
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}()
		}
		wg.Wait()
		cancel()
	}

	type workerStats struct {
		requests int64
		errors   int64
		non200   int64
		minLat   time.Duration
		maxLat   time.Duration
		samples  []time.Duration
	}

	stats := make([]workerStats, concurrency)
	runCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	start := time.Now()
	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			st := &stats[idx]
			st.minLat = time.Duration(math.MaxInt64)
			st.samples = make([]time.Duration, 0, min(maxSamplesPerWorker, 4096))
			var sampleCounter uint64

			for runCtx.Err() == nil {
				req, err := http.NewRequestWithContext(runCtx, http.MethodGet, targetURL, nil)
				if err != nil {
					return
				}
				t0 := time.Now()
				resp, err := client.Do(req)
				lat := time.Since(t0)
				if err != nil {
					// Ignore errors caused by the measurement window ending.
					if runCtx.Err() != nil {
						return
					}
					st.errors++
					continue
				}
				_, copyErr := io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if copyErr != nil {
					if runCtx.Err() != nil {
						return
					}
					st.errors++
					continue
				}
				st.requests++
				if resp.StatusCode != http.StatusOK {
					st.non200++
				}
				if lat < st.minLat {
					st.minLat = lat
				}
				if lat > st.maxLat {
					st.maxLat = lat
				}
				sampleCounter++
				if len(st.samples) < maxSamplesPerWorker {
					st.samples = append(st.samples, lat)
				} else {
					// Deterministic stride replacement keeps memory bounded with zero RNG overhead.
					pos := sampleCounter % maxSamplesPerWorker
					st.samples[pos] = lat
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	res := LoadResult{
		Tool:        "go-loadgen",
		Concurrency: concurrency,
		Duration:    elapsed,
	}
	minLat := time.Duration(math.MaxInt64)
	var maxLat time.Duration
	totalSamples := 0
	for i := range stats {
		res.Requests += stats[i].requests
		res.Errors += stats[i].errors
		res.Non200 += stats[i].non200
		if stats[i].requests > 0 {
			if stats[i].minLat < minLat {
				minLat = stats[i].minLat
			}
			if stats[i].maxLat > maxLat {
				maxLat = stats[i].maxLat
			}
		}
		totalSamples += len(stats[i].samples)
	}
	if elapsed > 0 {
		res.ReqPerSec = float64(res.Requests) / elapsed.Seconds()
	}
	if totalSamples > 0 {
		all := make([]time.Duration, 0, totalSamples)
		for i := range stats {
			all = append(all, stats[i].samples...)
		}
		slices.Sort(all)
		res.LatencyMin = minLat
		res.LatencyMax = maxLat
		res.LatencyP50 = percentile(all, 0.50)
		res.LatencyP95 = percentile(all, 0.95)
		res.LatencyP99 = percentile(all, 0.99)
	}
	return res, nil
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// detectExternalLoadTool returns the name and path of oha or wrk if installed.
func detectExternalLoadTool() (name, path string) {
	for _, candidate := range []string{"oha", "wrk"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return candidate, p
		}
	}
	return "", ""
}

// runExternalLoad runs oha or wrk against targetURL.
func runExternalLoad(ctx context.Context, toolName, toolPath, targetURL string, concurrency int, duration time.Duration) (LoadResult, error) {
	secs := max(1, int(math.Round(duration.Seconds())))
	switch toolName {
	case "oha":
		args := []string{
			"--no-tui",
			"--output-format", "json",
			"-c", strconv.Itoa(concurrency),
			"-z", fmt.Sprintf("%ds", secs),
			targetURL,
		}
		out, err := exec.CommandContext(ctx, toolPath, args...).CombinedOutput()
		if err != nil {
			return LoadResult{}, fmt.Errorf("oha failed: %w\n%s", err, string(out))
		}
		res, err := parseOhaJSON(out, concurrency, duration)
		if err != nil {
			return LoadResult{}, err
		}
		return res, nil

	case "wrk":
		threads := min(max(1, runtime.NumCPU()), concurrency)
		args := []string{
			"-t", strconv.Itoa(threads),
			"-c", strconv.Itoa(concurrency),
			"-d", fmt.Sprintf("%ds", secs),
			targetURL,
		}
		out, err := exec.CommandContext(ctx, toolPath, args...).CombinedOutput()
		if err != nil {
			return LoadResult{}, fmt.Errorf("wrk failed: %w\n%s", err, string(out))
		}
		res, err := parseWrkOutput(string(out), concurrency, duration)
		if err != nil {
			return LoadResult{}, err
		}
		return res, nil

	default:
		return LoadResult{}, fmt.Errorf("unsupported external load tool %q", toolName)
	}
}

type ohaReport struct {
	Summary struct {
		RequestsPerSec float64 `json:"requestsPerSec"`
		Total          float64 `json:"total"`
		Fastest        float64 `json:"fastest"`
		Slowest        float64 `json:"slowest"`
	} `json:"summary"`
	LatencyPercentiles map[string]float64 `json:"latencyPercentiles"`
	StatusCodeDist     map[string]int64   `json:"statusCodeDistribution"`
}

func parseOhaJSON(data []byte, concurrency int, fallbackDur time.Duration) (LoadResult, error) {
	var rep ohaReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return LoadResult{}, fmt.Errorf("parse oha json: %w", err)
	}
	res := LoadResult{
		Tool:        "oha",
		Concurrency: concurrency,
		Duration:    fallbackDur,
		ReqPerSec:   rep.Summary.RequestsPerSec,
		LatencyMin:  secondsToDuration(rep.Summary.Fastest),
		LatencyMax:  secondsToDuration(rep.Summary.Slowest),
		LatencyP50:  secondsToDuration(rep.LatencyPercentiles["p50"]),
		LatencyP95:  secondsToDuration(rep.LatencyPercentiles["p95"]),
		LatencyP99:  secondsToDuration(rep.LatencyPercentiles["p99"]),
	}
	if rep.Summary.Total > 0 {
		res.Duration = secondsToDuration(rep.Summary.Total)
	}
	for code, count := range rep.StatusCodeDist {
		res.Requests += count
		if code != "200" {
			res.Non200 += count
		}
	}
	return res, nil
}

func secondsToDuration(sec float64) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

var (
	wrkReqSecRe   = regexp.MustCompile(`Requests/sec:\s+([0-9]+(?:\.[0-9]+)?)`)
	wrkTotalReqRe = regexp.MustCompile(`([0-9]+)\s+requests\s+in\s+`)
)

func parseWrkOutput(out string, concurrency int, duration time.Duration) (LoadResult, error) {
	m := wrkReqSecRe.FindStringSubmatch(out)
	if len(m) < 2 {
		return LoadResult{}, fmt.Errorf("could not parse Requests/sec from wrk output:\n%s", out)
	}
	rps, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return LoadResult{}, fmt.Errorf("parse wrk Requests/sec %q: %w", m[1], err)
	}
	res := LoadResult{
		Tool:        "wrk",
		Concurrency: concurrency,
		Duration:    duration,
		ReqPerSec:   rps,
		RawOutput:   strings.TrimSpace(out),
	}
	if tm := wrkTotalReqRe.FindStringSubmatch(out); len(tm) >= 2 {
		if n, err := strconv.ParseInt(tm[1], 10, 64); err == nil {
			res.Requests = n
		}
	}
	return res, nil
}

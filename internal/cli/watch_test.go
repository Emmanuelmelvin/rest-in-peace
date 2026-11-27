package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	bin      string
	env      []string
	done     chan struct{}
	stopOnce sync.Once

	mu       sync.Mutex
	stopped  bool
	graceful bool
}

func newFakeProcess(bin string, env []string) *fakeProcess {
	return &fakeProcess{
		bin:  bin,
		env:  append([]string(nil), env...),
		done: make(chan struct{}),
	}
}

func (p *fakeProcess) Done() <-chan struct{} {
	return p.done
}

func (p *fakeProcess) Stop(graceful bool) {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.graceful = graceful
		p.mu.Unlock()
		close(p.done)
	})
}

func (p *fakeProcess) isStopped() (stopped, graceful bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped, p.graceful
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotAndDiffWatched(t *testing.T) {
	root := t.TempDir()

	// Watched files
	mustWriteFile(t, filepath.Join(root, "main.bas"), "PRINT 1\n")
	mustWriteFile(t, filepath.Join(root, "sub", "billing.bas"), "EXPORT CONST TAX = 10\n")
	mustWriteFile(t, filepath.Join(root, ".env"), "PORT=8080\n")
	mustWriteFile(t, filepath.Join(root, "migrations", "001_init.sql"), "CREATE TABLE t (id INT);\n")

	// Ignored files and directories
	mustWriteFile(t, filepath.Join(root, "README.md"), "ignore me\n")
	mustWriteFile(t, filepath.Join(root, ".rip", "build", "generated.bas"), "ignore\n")
	mustWriteFile(t, filepath.Join(root, ".git", "hook.bas"), "ignore\n")
	mustWriteFile(t, filepath.Join(root, "dist", "out.bas"), "ignore\n")
	mustWriteFile(t, filepath.Join(root, "node_modules", "pkg", "x.bas"), "ignore\n")

	snap1, err := snapshotWatched(root)
	if err != nil {
		t.Fatalf("snapshotWatched: %v", err)
	}

	wantKeys := []string{".env", "main.bas", "migrations/001_init.sql", "sub/billing.bas"}
	if len(snap1) != len(wantKeys) {
		t.Fatalf("snapshot keys = %v, want %v", snap1, wantKeys)
	}
	for _, k := range wantKeys {
		if _, ok := snap1[k]; !ok {
			t.Errorf("missing expected watched key %q", k)
		}
	}

	// Modify nested .bas, add migration, remove .env, and touch an ignored file.
	mustWriteFile(t, filepath.Join(root, "sub", "billing.bas"), "EXPORT CONST TAX = 15\n")
	mustWriteFile(t, filepath.Join(root, "migrations", "002_items.sql"), "CREATE TABLE items (id INT);\n")
	if err := os.Remove(filepath.Join(root, ".env")); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(root, "README.md"), "still ignored\n")

	snap2, err := snapshotWatched(root)
	if err != nil {
		t.Fatalf("snapshotWatched 2: %v", err)
	}

	got := diffSnapshots(snap1, snap2)
	want := []string{".env", "migrations/002_items.sql", "sub/billing.bas"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("diffSnapshots = %v, want %v", got, want)
	}
}

func TestWatchLoop_Debounce(t *testing.T) {
	root := t.TempDir()
	mainBas := filepath.Join(root, "main.bas")
	mustWriteFile(t, mainBas, "PRINT 1\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu         sync.Mutex
		buildCount int
	)
	buildCh := make(chan int, 10)
	var stderr syncBuffer

	cfg := watcherConfig{
		root:         root,
		pollInterval: 15 * time.Millisecond,
		debounce:     80 * time.Millisecond,
		stderr:       &stderr,
		build: func(ctx context.Context, gen int) (string, bool) {
			mu.Lock()
			buildCount++
			n := buildCount
			mu.Unlock()
			buildCh <- n
			return fmt.Sprintf("fake-bin-%d", gen), true
		},
		start: func(bin string, extraEnv []string) (devProcess, error) {
			return newFakeProcess(bin, extraEnv), nil
		},
		removeBin: func(string) {},
	}

	done := make(chan int, 1)
	go func() {
		done <- runWatchLoop(ctx, cfg)
	}()

	// Wait for initial build.
	select {
	case n := <-buildCh:
		if n != 1 {
			t.Fatalf("first build = %d, want 1", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial build")
	}

	// Burst of 3 rapid edits within the debounce window should coalesce into 1 rebuild.
	for i := 2; i <= 4; i++ {
		mustWriteFile(t, mainBas, fmt.Sprintf("PRINT %d\n", i))
		time.Sleep(25 * time.Millisecond)
	}

	select {
	case n := <-buildCh:
		if n != 2 {
			t.Fatalf("debounced build = %d, want 2", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced rebuild")
	}

	// Ensure no extra rebuild fires after settling.
	select {
	case n := <-buildCh:
		t.Fatalf("unexpected extra build #%d after debounce", n)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watch loop shutdown")
	}
}

func TestWatchLoop_KeepLastGoodBuildOnFailure(t *testing.T) {
	root := t.TempDir()
	mainBas := filepath.Join(root, "main.bas")
	mustWriteFile(t, mainBas, "PRINT \"v1\"\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu      sync.Mutex
		procs   []*fakeProcess
		removed []string
	)
	buildCh := make(chan int, 10)
	startCh := make(chan *fakeProcess, 10)
	var stderr syncBuffer

	cfg := watcherConfig{
		root:         root,
		pollInterval: 15 * time.Millisecond,
		debounce:     40 * time.Millisecond,
		stderr:       &stderr,
		build: func(ctx context.Context, gen int) (string, bool) {
			defer func() { buildCh <- gen }()
			if gen == 2 {
				fmt.Fprintln(&stderr, "?SYNTAX ERROR IN main.bas:1 — unexpected token")
				return "", false
			}
			return fmt.Sprintf("fake-bin-%d", gen), true
		},
		start: func(bin string, extraEnv []string) (devProcess, error) {
			p := newFakeProcess(bin, extraEnv)
			mu.Lock()
			procs = append(procs, p)
			mu.Unlock()
			startCh <- p
			return p, nil
		},
		removeBin: func(bin string) {
			mu.Lock()
			removed = append(removed, bin)
			mu.Unlock()
		},
	}

	done := make(chan int, 1)
	go func() {
		done <- runWatchLoop(ctx, cfg)
	}()

	// Gen 1: succeeds and starts proc1.
	<-buildCh
	proc1 := <-startCh
	if proc1.bin != "fake-bin-1" {
		t.Fatalf("proc1.bin = %q, want fake-bin-1", proc1.bin)
	}

	// Edit file -> Gen 2 fails; proc1 MUST stay running.
	mustWriteFile(t, mainBas, "BROKEN SYNTAX\n")
	select {
	case gen := <-buildCh:
		if gen != 2 {
			t.Fatalf("gen = %d, want 2", gen)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for failing build")
	}

	// Give the loop a moment to process the failed build result.
	time.Sleep(40 * time.Millisecond)
	if stopped, _ := proc1.isStopped(); stopped {
		t.Fatal("proc1 was stopped on build failure; expected last good build to keep running")
	}
	logs := stderr.String()
	if !strings.Contains(logs, "?SYNTAX ERROR IN main.bas:1") {
		t.Errorf("stderr missing classic diagnostic:\n%s", logs)
	}
	if !strings.Contains(logs, "keeping last good build running") {
		t.Errorf("stderr missing last-good-build notice:\n%s", logs)
	}

	// Fix file -> Gen 3 succeeds; proc1 is stopped (non-graceful restart), proc3 starts.
	mustWriteFile(t, mainBas, "PRINT \"v3\"\n")
	select {
	case gen := <-buildCh:
		if gen != 3 {
			t.Fatalf("gen = %d, want 3", gen)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovery build")
	}
	proc3 := <-startCh
	if proc3.bin != "fake-bin-3" {
		t.Fatalf("proc3.bin = %q, want fake-bin-3", proc3.bin)
	}
	if stopped, graceful := proc1.isStopped(); !stopped || graceful {
		t.Fatalf("proc1 stopped=%v graceful=%v, want stopped=true graceful=false", stopped, graceful)
	}

	// Cancel context (Ctrl+C) -> proc3 stopped cleanly (graceful=true).
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}

	if stopped, graceful := proc3.isStopped(); !stopped || !graceful {
		t.Fatalf("proc3 stopped=%v graceful=%v, want stopped=true graceful=true", stopped, graceful)
	}

	mu.Lock()
	gotRemoved := strings.Join(removed, ",")
	mu.Unlock()
	if gotRemoved != "fake-bin-1,fake-bin-3" {
		t.Errorf("removed binaries = %q, want fake-bin-1,fake-bin-3", gotRemoved)
	}
}

func TestWatchLoop_InitialFailureThenRecoverAndEnvReload(t *testing.T) {
	root := t.TempDir()
	mainBas := filepath.Join(root, "main.bas")
	envFile := filepath.Join(root, ".env")
	mustWriteFile(t, mainBas, "BROKEN\n")
	mustWriteFile(t, envFile, "PORT=8081\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buildCh := make(chan int, 10)
	startCh := make(chan *fakeProcess, 10)
	var stderr syncBuffer

	cfg := watcherConfig{
		root:         root,
		pollInterval: 15 * time.Millisecond,
		debounce:     40 * time.Millisecond,
		stderr:       &stderr,
		build: func(ctx context.Context, gen int) (string, bool) {
			defer func() { buildCh <- gen }()
			if gen == 1 {
				fmt.Fprintln(&stderr, "?SYNTAX ERROR IN main.bas:1 — bad initial code")
				return "", false
			}
			return fmt.Sprintf("fake-bin-%d", gen), true
		},
		start: func(bin string, extraEnv []string) (devProcess, error) {
			p := newFakeProcess(bin, extraEnv)
			startCh <- p
			return p, nil
		},
		loadEnv: func() []string {
			return loadDotEnv(envFile)
		},
		removeBin: func(string) {},
	}

	done := make(chan int, 1)
	go func() {
		done <- runWatchLoop(ctx, cfg)
	}()

	// Gen 1 fails initially.
	if gen := <-buildCh; gen != 1 {
		t.Fatalf("gen = %d, want 1", gen)
	}
	time.Sleep(30 * time.Millisecond)
	if !strings.Contains(stderr.String(), "waiting for changes") {
		t.Errorf("stderr missing waiting-for-changes message:\n%s", stderr.String())
	}

	// Fix main.bas -> Gen 2 succeeds with PORT=8081.
	mustWriteFile(t, mainBas, "PRINT \"fixed\"\n")
	<-buildCh
	proc2 := <-startCh
	if !strings.Contains(strings.Join(proc2.env, ","), "PORT=8081") {
		t.Fatalf("proc2 env = %v, want PORT=8081", proc2.env)
	}

	// Edit .env -> Gen 3 reloads PORT=9090.
	mustWriteFile(t, envFile, "PORT=9090\n")
	<-buildCh
	proc3 := <-startCh
	if !strings.Contains(strings.Join(proc3.env, ","), "PORT=9090") {
		t.Fatalf("proc3 env = %v, want PORT=9090", proc3.env)
	}

	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

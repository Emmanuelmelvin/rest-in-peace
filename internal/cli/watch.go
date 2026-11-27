package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultPollInterval = 200 * time.Millisecond
	defaultDebounce     = 150 * time.Millisecond
	defaultStopTimeout  = 3 * time.Second
)

// fileStamp tracks a watched file's modification time and size so edits within
// coarse filesystem timestamp granularity are still detected.
type fileStamp struct {
	modTime time.Time
	size    int64
}

// devProcess represents a running child process managed by `rip dev`.
type devProcess interface {
	Done() <-chan struct{}
	Stop(graceful bool)
}

// devBuilder compiles and builds the project into a runnable binary path.
// Diagnostics are written to stderr; returning ("", false) signals failure.
type devBuilder func(ctx context.Context, gen int) (bin string, ok bool)

// devStarter launches a built binary with the given extra environment variables.
type devStarter func(bin string, extraEnv []string) (devProcess, error)

type watcherConfig struct {
	root         string
	pollInterval time.Duration
	debounce     time.Duration
	stderr       io.Writer
	build        devBuilder
	start        devStarter
	loadEnv      func() []string
	removeBin    func(string)
}

// skippedWatchDirs are directories under the project root that never contain
// user source files and should not be scanned by `rip dev`.
var skippedWatchDirs = map[string]bool{
	".rip":         true,
	".git":         true,
	"dist":         true,
	"node_modules": true,
}

// snapshotWatched scans root for all *.bas files recursively, the project's
// .env file, and all files under migrations/.
func snapshotWatched(root string) (map[string]fileStamp, error) {
	snap := make(map[string]fileStamp)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			// Ignore transient errors (e.g. temp files vanishing mid-save).
			return nil
		}
		if d.IsDir() {
			if path != root && skippedWatchDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if !isWatchedFile(relSlash, d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		snap[relSlash] = fileStamp{
			modTime: info.ModTime(),
			size:    info.Size(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

func isWatchedFile(relSlash, baseName string) bool {
	if strings.HasSuffix(strings.ToLower(baseName), ".bas") {
		return true
	}
	if relSlash == ".env" {
		return true
	}
	if strings.HasPrefix(relSlash, "migrations/") {
		return true
	}
	return false
}

// diffSnapshots returns the sorted relative paths of files that were added,
// removed, or modified between prev and curr.
func diffSnapshots(prev, curr map[string]fileStamp) []string {
	var changed []string
	for k, v := range curr {
		if old, ok := prev[k]; !ok || old != v {
			changed = append(changed, k)
		}
	}
	for k := range prev {
		if _, ok := curr[k]; !ok {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

func formatChangeReason(files []string) string {
	switch len(files) {
	case 0:
		return "change detected"
	case 1, 2, 3:
		return fmt.Sprintf("change detected (%s)", strings.Join(files, ", "))
	default:
		return fmt.Sprintf("change detected (%s, +%d more)", strings.Join(files[:2], ", "), len(files)-2)
	}
}

// runWatchLoop runs the initial build and then polls for file changes until
// ctx is cancelled. When a rebuild fails, the last good build stays running.
func runWatchLoop(ctx context.Context, cfg watcherConfig) int {
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = defaultPollInterval
	}
	if cfg.debounce <= 0 {
		cfg.debounce = defaultDebounce
	}
	if cfg.stderr == nil {
		cfg.stderr = os.Stderr
	}
	if cfg.loadEnv == nil {
		cfg.loadEnv = func() []string { return nil }
	}
	if cfg.removeBin == nil {
		cfg.removeBin = func(p string) { _ = os.Remove(p) }
	}

	lastSnap, err := snapshotWatched(cfg.root)
	if err != nil {
		fmt.Fprintln(cfg.stderr, err)
		return 1
	}

	var (
		gen         int
		currentProc devProcess
		currentBin  string
	)

	isRunning := func(p devProcess) bool {
		if p == nil {
			return false
		}
		select {
		case <-p.Done():
			return false
		default:
			return true
		}
	}

	rebuild := func(reason string) {
		gen++
		if reason != "" {
			fmt.Fprintf(cfg.stderr, "rip dev: %s — rebuilding...\n", reason)
		}
		bin, ok := cfg.build(ctx, gen)
		if ctx.Err() != nil {
			if ok && bin != "" && bin != currentBin {
				cfg.removeBin(bin)
			}
			return
		}
		if !ok {
			if isRunning(currentProc) {
				fmt.Fprintln(cfg.stderr, "rip dev: build failed — keeping last good build running")
			} else {
				fmt.Fprintln(cfg.stderr, "rip dev: build failed — waiting for changes...")
			}
			return
		}

		if currentProc != nil {
			currentProc.Stop(false)
			currentProc = nil
		}
		if currentBin != "" && currentBin != bin {
			cfg.removeBin(currentBin)
		}
		currentBin = bin

		proc, err := cfg.start(bin, cfg.loadEnv())
		if err != nil {
			fmt.Fprintf(cfg.stderr, "rip dev: failed to start program: %v\n", err)
			return
		}
		currentProc = proc
	}

	rebuild("")
	if ctx.Err() != nil {
		if currentProc != nil {
			currentProc.Stop(true)
		}
		if currentBin != "" {
			cfg.removeBin(currentBin)
		}
		return 0
	}

	ticker := time.NewTicker(cfg.pollInterval)
	defer ticker.Stop()

	var (
		debounceTimer  *time.Timer
		debounceC      <-chan time.Time
		pendingChanges = make(map[string]struct{})
	)

	stopDebounce := func() {
		if debounceTimer != nil {
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer = nil
			debounceC = nil
		}
	}
	defer stopDebounce()

	for {
		select {
		case <-ctx.Done():
			if currentProc != nil {
				currentProc.Stop(true)
			}
			if currentBin != "" {
				cfg.removeBin(currentBin)
			}
			return 0

		case <-ticker.C:
			curr, err := snapshotWatched(cfg.root)
			if err != nil {
				continue
			}
			changed := diffSnapshots(lastSnap, curr)
			if len(changed) == 0 {
				continue
			}
			lastSnap = curr
			for _, f := range changed {
				pendingChanges[f] = struct{}{}
			}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(cfg.debounce)
				debounceC = debounceTimer.C
			} else {
				if !debounceTimer.Stop() {
					select {
					case <-debounceTimer.C:
					default:
					}
				}
				debounceTimer.Reset(cfg.debounce)
			}

		case <-debounceC:
			debounceTimer = nil
			debounceC = nil
			files := make([]string, 0, len(pendingChanges))
			for f := range pendingChanges {
				files = append(files, f)
			}
			sort.Strings(files)
			clear(pendingChanges)
			rebuild(formatChangeReason(files))
		}
	}
}

type osProcess struct {
	cmd         *exec.Cmd
	done        chan struct{}
	stopTimeout time.Duration
	stopOnce    sync.Once
}

func startOSProcess(bin string, extraEnv []string) (devProcess, error) {
	cmd := exec.Command(bin)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), extraEnv...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &osProcess{
		cmd:         cmd,
		done:        make(chan struct{}),
		stopTimeout: defaultStopTimeout,
	}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *osProcess) Done() <-chan struct{} {
	return p.done
}

func (p *osProcess) Stop(graceful bool) {
	p.stopOnce.Do(func() {
		select {
		case <-p.done:
			return
		default:
		}

		if runtime.GOOS == "windows" {
			if graceful {
				// On Ctrl+C, Windows delivers CTRL_C_EVENT to the child too;
				// allow a brief window for graceful shutdown before forcing exit.
				timer := time.NewTimer(500 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-p.done:
					return
				case <-timer.C:
				}
			}
			_ = p.cmd.Process.Kill()
			<-p.done
			return
		}

		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			_ = p.cmd.Process.Kill()
			<-p.done
			return
		}
		timer := time.NewTimer(p.stopTimeout)
		defer timer.Stop()
		select {
		case <-p.done:
			return
		case <-timer.C:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
}

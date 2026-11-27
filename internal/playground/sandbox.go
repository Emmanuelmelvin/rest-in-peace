package playground

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// startupTimeout is how long a program's top-level code gets to reach SERVE
// (or end). Top-level code has no request deadline, so this is its loop guard.
var startupTimeout = 10 * time.Second

// crashLoopLimit restarts within crashLoopWindow stop further restarts.
const (
	crashLoopLimit  = 5
	crashLoopWindow = time.Minute
)

// readyPrefix is what rt.App.Run prints once it is listening.
const readyPrefix = "READY. listening on :"

type sbState int

const (
	stateStopped sbState = iota // not started, or stopped to make room: starts on the next request
	stateRunning                // serving on its port
	stateEnded                  // ran to the end without SERVE, or failed to start
	stateCrashed                // died while serving: restarts on the next request
)

var (
	errNotServing = errors.New("THIS PROGRAM IS NOT SERVING — it ended without SERVE; check the log")
	errCrashLoop  = errors.New("THIS PROGRAM KEEPS DYING — fix it and RUN again")
	errStartLoop  = errors.New("TRAPPED IN AN ETERNAL GOTO LOOP")
)

// Sandbox is one running playground program: a separate OS process (design §5).
// The host is expected to run on Linux in a container; rlimits are Linux-only.
type Sandbox struct {
	ID      string
	Created time.Time

	bin     string // the program's binary
	dataDir string // working directory, holding its SQLite file
	limits  Limits
	logs    *logRing

	startMu sync.Mutex // serialises starting and stopping the process

	mu       sync.Mutex
	port     int
	state    sbState
	cmd      *exec.Cmd     // nil when no process is running
	exited   chan struct{} // closed when cmd has exited
	lastUsed time.Time
	starts   []time.Time // recent starts, for the crash-loop guard
}

func newSandbox(id, bin, dataDir string, created time.Time, limits Limits) *Sandbox {
	return &Sandbox{
		ID: id, Created: created,
		bin: bin, dataDir: dataDir, limits: limits,
		logs:     newLogRing(),
		lastUsed: time.Now(),
	}
}

// Env returns the complete environment a program process gets. It never
// inherits the host's environment (security.md).
func Env(port int, dataDir string, limits Limits) []string {
	soft := limits.MemoryMB * 80 / 100 // leave headroom below the hard cap
	return []string{
		fmt.Sprintf("PORT=%d", port),
		"RIP_PLAYGROUND=1",
		fmt.Sprintf("GOMEMLIMIT=%dMiB", soft),
		"GOMAXPROCS=1",
		"DATABASE_URL=sqlite://" + filepath.Join(dataDir, "program.db"),
	}
}

// Port returns the port the program listens on (0 when it isn't running).
func (sb *Sandbox) Port() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.state != stateRunning {
		return 0
	}
	return sb.port
}

func (sb *Sandbox) currentState() sbState {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.state
}

func (sb *Sandbox) touch() {
	sb.mu.Lock()
	sb.lastUsed = time.Now()
	sb.mu.Unlock()
}

func (sb *Sandbox) lastUse() time.Time {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.lastUsed
}

// target returns the port and process to proxy to, if the program is serving.
func (sb *Sandbox) target() (int, *exec.Cmd, bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.state != stateRunning || sb.cmd == nil {
		return 0, nil, false
	}
	return sb.port, sb.cmd, true
}

// ensureRunning starts the program if it was stopped to make room or died
// while serving. It returns errNotServing for a program that never serves.
func (sb *Sandbox) ensureRunning() error {
	sb.startMu.Lock()
	defer sb.startMu.Unlock()
	sb.mu.Lock()
	state := sb.state
	recent := 0
	for _, t := range sb.starts {
		if time.Since(t) < crashLoopWindow {
			recent++
		}
	}
	sb.mu.Unlock()
	switch state {
	case stateRunning:
		return nil
	case stateEnded:
		return errNotServing
	case stateCrashed:
		if recent >= crashLoopLimit {
			return errCrashLoop
		}
		sb.logs.add("host", "RESTARTING AFTER A CRASH…")
	}
	return sb.startLocked()
}

// start runs the program and waits until it is serving, has ended, or has
// used up startupTimeout. A program that ends without SERVE is not an error.
func (sb *Sandbox) start() error {
	sb.startMu.Lock()
	defer sb.startMu.Unlock()
	return sb.startLocked()
}

func (sb *Sandbox) startLocked() error {
	sb.stopLocked(stateStopped)

	port, err := freePort()
	if err != nil {
		return err
	}
	ready := make(chan int, 1)
	stdout := &lineWriter{stream: "stdout", ring: sb.logs, onLine: func(line string) {
		if p, ok := readyPort(line); ok {
			select {
			case ready <- p:
			default:
			}
		}
	}}
	stderr := &lineWriter{stream: "stderr", ring: sb.logs}

	cmd := exec.Command(sb.bin)
	cmd.Env = Env(port, sb.dataDir, sb.limits)
	cmd.Dir = sb.dataDir
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.SysProcAttr = sysProcAttr()
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("?INTERNAL ERROR — the program could not be started: %v", err)
	}
	if err := applyLimits(cmd.Process.Pid, sb.limits); err != nil {
		killProcess(cmd)
		_ = cmd.Wait()
		return fmt.Errorf("?INTERNAL ERROR — the program's memory and CPU caps could not be set: %v", err)
	}

	exited := make(chan struct{})
	sb.mu.Lock()
	sb.cmd, sb.exited, sb.port, sb.state = cmd, exited, port, stateStopped
	sb.starts = append(sb.starts, time.Now())
	if len(sb.starts) > crashLoopLimit {
		sb.starts = sb.starts[len(sb.starts)-crashLoopLimit:]
	}
	sb.mu.Unlock()

	go func() {
		err := cmd.Wait()
		stdout.flush()
		stderr.flush()
		sb.mu.Lock()
		ours := sb.cmd == cmd // false when the host stopped it on purpose
		if ours {
			sb.cmd = nil
			if sb.state == stateRunning {
				sb.state = stateCrashed
			} else {
				sb.state = stateEnded
			}
		}
		sb.mu.Unlock()
		if ours {
			sb.logs.add("host", exitNote(err))
		}
		close(exited)
	}()

	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	timedOut := func() error {
		sb.stopLocked(stateEnded)
		sb.logs.add("host", fmt.Sprintf("?ETERNAL LOOP ERROR — the top-level code did not reach SERVE or END within %s, so the program was killed", startupTimeout))
		return errStartLoop
	}
	select {
	case p := <-ready:
		if p != port {
			sb.stopLocked(stateEnded)
			msg := fmt.Sprintf("?SERVE ERROR — in the playground, write SERVE without a port; the host picks it (got %d)", p)
			sb.logs.add("host", msg)
			return errors.New(msg)
		}
		switch waitListening(port, exited, deadline.C) {
		case listenTimedOut:
			return timedOut()
		case listenExited:
			return nil // it said READY and then ended; the log says why
		}
		sb.mu.Lock()
		if sb.cmd == cmd {
			sb.state = stateRunning
		}
		sb.mu.Unlock()
		return nil
	case <-exited:
		return nil // ran to the end without SERVE: its output is in the log
	case <-deadline.C:
		return timedOut()
	}
}

// Stop kills the process. The program starts again on its next request.
func (sb *Sandbox) Stop() {
	sb.startMu.Lock()
	defer sb.startMu.Unlock()
	sb.stopLocked(stateStopped)
}

// stopLocked kills the process, if any, waits for it, and sets the state.
// sb.startMu must be held.
func (sb *Sandbox) stopLocked(next sbState) {
	sb.mu.Lock()
	cmd, exited := sb.cmd, sb.exited
	sb.cmd = nil
	sb.state = next
	sb.mu.Unlock()
	if cmd != nil {
		killProcess(cmd)
		<-exited
	}
}

// restartIf kills and restarts the program, but only if cmd is still the
// process that is running (so two stuck requests cause one restart).
func (sb *Sandbox) restartIf(cmd *exec.Cmd, note string) {
	sb.startMu.Lock()
	defer sb.startMu.Unlock()
	sb.mu.Lock()
	same := sb.cmd == cmd
	sb.mu.Unlock()
	if !same {
		return
	}
	sb.logs.add("host", note)
	if err := sb.startLocked(); err != nil {
		sb.logs.add("host", "?RESTART FAILED — "+err.Error())
	} else if sb.currentState() == stateRunning {
		sb.logs.add("host", "RESTARTED. READY.")
	}
}

// remove stops the program and ends its log subscriptions.
func (sb *Sandbox) remove() {
	sb.Stop()
	sb.logs.add("host", "THIS PROGRAM HAS PASSED ON (EXPIRED).")
	sb.logs.close()
}

func readyPort(line string) (int, bool) {
	rest, ok := strings.CutPrefix(line, readyPrefix)
	if !ok {
		return 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(rest))
	return p, err == nil
}

const (
	listenOK = iota
	listenExited
	listenTimedOut
)

// waitListening dials the port until it accepts, the process exits or the
// deadline fires.
func waitListening(port int, exited <-chan struct{}, deadline <-chan time.Time) int {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return listenOK
		}
		select {
		case <-exited:
			return listenExited
		case <-deadline:
			return listenTimedOut
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func exitNote(err error) string {
	if err == nil {
		return "END. (THE PROGRAM FINISHED)"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ExitCode() >= 0 {
			return fmt.Sprintf("?PROGRAM DIED — exit status %d", ee.ExitCode())
		}
		return "?PROGRAM DIED — " + ee.String() + " (memory or CPU cap?)"
	}
	return "?PROGRAM DIED — " + err.Error()
}

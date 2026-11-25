package playground

import (
	"fmt"
	"path/filepath"
)

// Sandbox is one running playground program: a separate OS process (design §5).
// The host is expected to run on Linux in a container; rlimits are Linux-only.
type Sandbox struct {
	ID   string
	Port int
	// TODO(task 5.3): *exec.Cmd, a log ring buffer, last-used time.
}

// Env returns the complete environment a program process gets. It never
// inherits the host's environment (security.md).
func Env(port int, dataDir string, limits Limits) []string {
	soft := limits.MemoryMB * 80 / 100 // leave headroom below the hard cap
	return []string{
		fmt.Sprintf("PORT=%d", port),
		"RIP_PLAYGROUND=1",
		fmt.Sprintf("GOMEMLIMIT=%dMiB", soft),
		"DATABASE_URL=sqlite://" + filepath.Join(dataDir, "program.db"),
	}
}

// TODO(task 5.3):
//   - Start: exec the binary with Env(...), with RLIMIT_AS and RLIMIT_CPU set via SysProcAttr or prlimit.
//   - Watchdog: if a request takes longer than RequestTimeout + 1 s, kill the
//     process, answer 508 {"error": "TRAPPED IN AN ETERNAL GOTO LOOP"}, and restart it.
//   - Stop: kill the process and remove its data directory once Expiry passes.

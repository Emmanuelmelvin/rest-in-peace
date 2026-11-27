//go:build linux

package playground

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// Go reserves a large address space at startup (about 1.3 GB of mostly
// PROT_NONE mappings on linux/amd64), so an RLIMIT_AS of 128 MB stops the
// runtime before main ("failed to reserve page summary memory"). The memory
// cap is therefore enforced in three layers:
//   - GOMEMLIMIT (in Env) at 80% of MemoryMB, which makes the GC work hard first;
//   - RLIMIT_DATA, which counts writable private mappings, so it caps the heap:
//     MemoryMB plus what the runtime and a rip binary need at idle;
//   - RLIMIT_AS as a backstop against address-space abuse, above the
//     runtime's own reservation.
const (
	dataOverheadMB = 96
	addressSpaceMB = 2560
	maxFileBytes   = 64 << 20 // the program's SQLite file and anything else it writes
	maxOpenFiles   = 256
)

func sysProcAttr() *syscall.SysProcAttr {
	// A process group, so killProcess takes any children with it, and
	// SIGKILL if the host dies first.
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}

type rlimit64 struct{ Cur, Max uint64 }

func prlimit(pid, resource int, v uint64) error {
	lim := rlimit64{Cur: v, Max: v}
	_, _, errno := syscall.RawSyscall6(syscall.SYS_PRLIMIT64, uintptr(pid), uintptr(resource),
		uintptr(unsafe.Pointer(&lim)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// applyLimits sets the rlimits of a started process. It runs right after
// exec, while the child is still in Go runtime start-up.
func applyLimits(pid int, l Limits) error {
	const mb = 1 << 20
	limits := []struct {
		resource int
		value    uint64
	}{
		{syscall.RLIMIT_AS, uint64(addressSpaceMB+l.MemoryMB) * mb},
		{syscall.RLIMIT_DATA, uint64(l.MemoryMB+dataOverheadMB) * mb},
		{syscall.RLIMIT_CPU, uint64(l.CPUSeconds)},
		{syscall.RLIMIT_FSIZE, maxFileBytes},
		{syscall.RLIMIT_NOFILE, maxOpenFiles},
		{syscall.RLIMIT_CORE, 0},
	}
	for _, x := range limits {
		if x.value == 0 && x.resource != syscall.RLIMIT_CORE {
			continue // a zero limit in Limits means "no cap"
		}
		if err := prlimit(pid, x.resource, x.value); err != nil {
			return err
		}
	}
	return nil
}

// killProcess kills the program and everything in its process group.
func killProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = cmd.Process.Kill()
	}
}

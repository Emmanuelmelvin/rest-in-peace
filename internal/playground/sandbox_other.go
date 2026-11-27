//go:build !linux

package playground

import (
	"os/exec"
	"syscall"
)

// The playground host runs on Linux (deploy/playground.Dockerfile). On other
// systems, for development and tests, programs still run as separate
// processes with the Env() environment and GOMEMLIMIT, but without rlimits.

func sysProcAttr() *syscall.SysProcAttr { return nil }

func applyLimits(pid int, l Limits) error { return nil }

func killProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

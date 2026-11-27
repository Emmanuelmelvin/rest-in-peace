//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// processRSSBytes returns the resident set size (RSS) of the process with the
// given PID, in bytes. On Linux it reads /proc/<pid>/status; on other Unix
// systems (such as macOS) it falls back to `ps -o rss= -p <pid>`.
func processRSSBytes(pid int) (uint64, error) {
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseUint(fields[1], 10, 64)
					if err != nil {
						return 0, fmt.Errorf("parse VmRSS %q: %w", fields[1], err)
					}
					return kb * 1024, nil
				}
			}
		}
	}

	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, fmt.Errorf("ps rss for pid %d: %w", pid, err)
	}
	kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ps rss %q: %w", string(out), err)
	}
	return kb * 1024, nil
}

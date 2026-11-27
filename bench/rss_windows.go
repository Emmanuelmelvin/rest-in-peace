//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	processVMRead                  = 0x0010
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procK32GetProcessMemInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// processRSSBytes returns the resident memory (WorkingSetSize on Windows) of
// the process with the given PID, in bytes.
func processRSSBytes(pid int) (uint64, error) {
	h, err := syscall.OpenProcess(processQueryLimitedInformation|processVMRead, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer syscall.CloseHandle(h)

	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	r1, _, callErr := procK32GetProcessMemInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.CB),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("K32GetProcessMemoryInfo(%d): %w", pid, callErr)
	}
	return uint64(pmc.WorkingSetSize), nil
}

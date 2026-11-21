//go:build !windows

package e2e

import (
	"os"
	"syscall"
)

func pidRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// See https://pkg.go.dev/os#FindProcess
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func stopDaemon(proc *os.Process, _ string) error {
	// daemon will shut down gracefully, emitting coverage data
	return proc.Signal(syscall.SIGTERM)
}

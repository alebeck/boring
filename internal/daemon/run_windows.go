//go:build windows

package daemon

import (
	"os/exec"

	"github.com/alebeck/boring/internal/log"
)

func runDaemonized(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	if err := cmd.Start(); err != nil {
		return err
	}
	log.Debugf("Daemon started with PID %d", cmd.Process.Pid)
	return nil
}

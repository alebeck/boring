//go:build linux || darwin

package daemon

import (
	"os/exec"
	"syscall"

	"github.com/alebeck/boring/internal/log"
)

func runDaemonized(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	log.Debugf("Daemon started with PID %d", cmd.Process.Pid)
	return nil
}

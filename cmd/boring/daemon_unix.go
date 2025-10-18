//go:build linux || darwin

package main

import (
	"os/exec"
	"syscall"
)

func launchDaemonOS(name string, arg ...string) (int, error) {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

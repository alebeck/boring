//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"github.com/alebeck/boring/internal/log"
)

const DETACHED_PROCESS = 0x00000008

func launchDaemonOS(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS,
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	log.Debugf("Daemon started with PID %d", cmd.Process.Pid)
	return nil
}

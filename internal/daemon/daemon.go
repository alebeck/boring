package daemon

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	SOCK          = "/tmp/boringd.sock"
	LOG_FILE      = "/tmp/boringd.log"
	EXEC          = "boringd"
	START_TIMEOUT = 2 * time.Second
)

type CommandKind int

const (
	Nop CommandKind = iota
	Open
	Close
)

type Command struct {
	Kind   CommandKind   `json:"kind"`
	Tunnel tunnel.Tunnel `json:"tunnel,omitempty"`
}

type Response struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// EnsureDaemonAndConnect starts the daemon if it is not already running
// and returns a connection to it. This function is blocking.
func EnsureAndConnect() (net.Conn, error) {
	timer := time.After(START_TIMEOUT)
	starting := false
	sleepTime := 2 * time.Millisecond

	for {
		select {
		case <-timer:
			return nil, fmt.Errorf("Daemon was not responsive after %v", START_TIMEOUT)
		default:
			if conn, err := Connect(); err == nil {
				return conn, nil
			}
			if !starting {
				if err := startDaemon(EXEC, SOCK, LOG_FILE); err != nil {
					return nil, fmt.Errorf("Failed to start daemon: %v", err)
				}
				starting = true
			}
			time.Sleep(sleepTime)
			sleepTime *= 2 // Exponential backoff
		}
	}
}

func Connect() (net.Conn, error) {
	return net.Dial("unix", SOCK)
}

func startDaemon(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	err := cmd.Start()
	if err != nil {
		return err
	}

	log.Debugf("Daemon started with PID %d", cmd.Process.Pid)
	return nil
}

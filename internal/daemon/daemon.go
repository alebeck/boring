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
	INIT_WAIT     = 2 * time.Millisecond
	START_TIMEOUT = 2 * time.Second
)

type CommandKind int

const (
	Nop CommandKind = iota
	Open
	Close
	List
)

var commandKindNames = map[CommandKind]string{
	Nop:   "Nop",
	Open:  "Open",
	Close: "Close",
	List:  "List",
}

func (k CommandKind) String() string {
	n, ok := commandKindNames[k]
	if !ok {
		return fmt.Sprintf("%d", int(k))
	}
	return n
}

type Command struct {
	Kind   CommandKind   `json:"kind"`
	Tunnel tunnel.Tunnel `json:"tunnel,omitempty"`
}

type Response struct {
	Success bool                     `json:"success"`
	Error   string                   `json:"error"`
	Tunnels map[string]tunnel.Tunnel `json:"tunnels"`
}

// Ensure starts the daemon if it is not already running.
// This function is blocking.
func Ensure() error {
	timer := time.After(START_TIMEOUT)
	starting := false
	sleepTime := INIT_WAIT

	for {
		select {
		case <-timer:
			return fmt.Errorf("Daemon was not responsive after %v", START_TIMEOUT)
		default:
			if conn, err := Connect(); err == nil {
				go func() { conn.Close() }()
				return nil
			}
			if !starting {
				if err := startDaemon(EXEC, SOCK, LOG_FILE); err != nil {
					return fmt.Errorf("Failed to start daemon: %v", err)
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

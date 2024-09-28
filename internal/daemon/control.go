package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	Flag           = "--daemon"
	defaultSock    = "/tmp/boringd.sock"
	defaultLogFile = "/tmp/boringd.log"
	initWait       = 2 * time.Millisecond
	startTimeout   = 2 * time.Second
)

type CmdKind int

const (
	Nop CmdKind = iota
	Open
	Close
	List
)

var cmdKindNames = map[CmdKind]string{
	Nop:   "Nop",
	Open:  "Open",
	Close: "Close",
	List:  "List",
}

var sock, logFile, executableFile string

func init() {
	if sock = os.Getenv("BORING_SOCK"); sock == "" {
		sock = defaultSock
	}
	if logFile = os.Getenv("BORING_LOG_FILE"); logFile == "" {
		logFile = defaultLogFile
	}
}

func (k CmdKind) String() string {
	n, ok := cmdKindNames[k]
	if !ok {
		return fmt.Sprintf("%d", int(k))
	}
	return n
}

type Cmd struct {
	Kind   CmdKind       `json:"kind"`
	Tunnel tunnel.Tunnel `json:"tunnel,omitempty"`
}

type Resp struct {
	Success bool                     `json:"success"`
	Error   string                   `json:"error"`
	Tunnels map[string]tunnel.Tunnel `json:"tunnels"`
}

// Ensure starts the daemon if it is not already running.
// This function is blocking.
func Ensure() error {
	timer := time.After(startTimeout)
	starting := false
	sleepTime := initWait

	for {
		select {
		case <-timer:
			return fmt.Errorf("Daemon was not responsive after %v", startTimeout)
		default:
			if conn, err := Connect(); err == nil {
				go func() { conn.Close() }()
				return nil
			}
			if !starting {
				if err := startDaemon(); err != nil {
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
	return net.Dial("unix", sock)
}

func startDaemon() error {
	ex, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %v", err)
	}

	cmd := exec.Command(ex, Flag)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		return err
	}

	log.Debugf("Daemon started with PID %d", cmd.Process.Pid)
	return nil
}

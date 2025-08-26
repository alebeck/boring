package e2e

import (
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
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

func killPID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// daemon will shut down gracefully, emitting coverage data
	return proc.Signal(syscall.SIGTERM)
}

func testDaemonLaunch(t *testing.T, env []string) {
	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	// debug output should contain the PID of the daemon
	re := regexp.MustCompile(`PID\s(\d+)`)
	match := re.FindStringSubmatch(out)
	if len(match) < 2 {
		t.Fatalf("PID not in output")
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("invalid PID: %v", err)
	}

	// verify daemon is running
	if !pidRunning(pid) {
		t.Fatalf("pid %d not running", pid)
	}

	if err := killPID(pid); err != nil {
		t.Fatalf("failed to kill daemon: %v", err)
	}

	// Finally check for graceful termination
	time.Sleep(50 * time.Millisecond)

	if pidRunning(pid) {
		t.Fatalf("pid %d running", pid)
	}

	sock := getEnv(env, "BORING_SOCK")
	if _, err = os.Stat(sock); err == nil {
		t.Fatalf("sock file exists after termination: %v", err)
	}
}

// Test that daemon is correctly started if not running
func TestDaemonLaunch(t *testing.T) {
	cfg := defaultConfig
	cfg.noSpawn = false
	cfg.debug = true
	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	testDaemonLaunch(t, env)
}

// Test that we can recover from a situation where the socket exists
// but is not bindable, this can happen after force shutdowns.
func TestDaemonLaunchBadSocket(t *testing.T) {
	cfg := defaultConfig
	cfg.noSpawn = false
	cfg.debug = true
	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	// Create non-bindable file
	var s string
	for _, e := range env {
		if strings.HasPrefix(e, "BORING_SOCK=") {
			s = strings.Split(e, "=")[1]
		}
	}
	if err = os.WriteFile(s, []byte("test"), 111); err != nil {
		t.Fatalf("could not create socket file: %v", err)
	}

	testDaemonLaunch(t, env)
}

func TestDaemonInvalidCommand(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Need this for IPC functions called below
	log.Init(io.Discard, false, false)

	cmd := daemon.Cmd{Kind: daemon.CmdKind(99)}
	conn, err := net.Dial("unix", getEnv(env, "BORING_SOCK"))
	if err != nil {
		t.Fatalf("could not connect to daemon")
	}
	defer conn.Close()

	if err = ipc.Write(cmd, conn); err != nil {
		t.Fatalf(err.Error())
	}
	var r daemon.Resp
	if err = ipc.Read(&r, conn); err != nil {
		t.Fatalf(err.Error())
	}

	if r.Success || !strings.Contains(r.Error, "unknown command") {
		t.Fatalf("did not get correct error message: %v", r.Error)
	}
}

func TestDaemonInvalidTunnel(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Need this for IPC functions called below
	log.Init(io.Discard, false, false)

	tun := tunnel.Desc{Name: "notrunning"}
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: tun}
	conn, err := net.Dial("unix", getEnv(env, "BORING_SOCK"))
	if err != nil {
		t.Fatalf("could not connect to daemon")
	}
	defer conn.Close()

	if err = ipc.Write(cmd, conn); err != nil {
		t.Fatalf(err.Error())
	}
	var r daemon.Resp
	if err = ipc.Read(&r, conn); err != nil {
		t.Fatalf(err.Error())
	}

	if r.Success || !strings.Contains(r.Error, "tunnel not running") {
		t.Fatalf("did not get correct error message: %v", r.Error)
	}
}

func TestDaemonFailSetupListener(t *testing.T) {
	// TODO
}

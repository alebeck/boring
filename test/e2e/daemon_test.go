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

func testDaemonLaunch(t *testing.T, env []string) string {
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

	// Finally check for graceful termination. Poll instead of sleeping a
	// fixed amount, since shutdown is slower under the race detector.
	deadline := time.Now().Add(2 * time.Second)
	for pidRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if pidRunning(pid) {
		t.Fatalf("pid %d running", pid)
	}

	sock := getEnv(env, "BORING_SOCK")
	if _, err = os.Stat(sock); err == nil {
		t.Fatalf("sock file exists after termination: %v", err)
	}

	return out
}

// Test that daemon is correctly started if not running
func TestDaemonLaunch(t *testing.T) {
	cfg := defaultConfig
	cfg.noSpawn = false
	cfg.debug = true
	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
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
		t.Fatalf("%v", err.Error())
	}

	// Create non-bindable file
	s := getEnv(env, "BORING_SOCK")
	if err = os.WriteFile(s, []byte("test"), 0o111); err != nil {
		t.Fatalf("could not create socket file: %v", err)
	}

	testDaemonLaunch(t, env)
}

func TestDaemonInvalidCommand(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
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
		t.Fatalf("%v", err.Error())
	}
	var r daemon.Resp
	if err = ipc.Read(&r, conn); err != nil {
		t.Fatalf("%v", err.Error())
	}

	if r.Success || !strings.Contains(r.Error, "unknown command") {
		t.Fatalf("did not get correct error message: %v", r.Error)
	}
}

func TestDaemonInvalidTunnel(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	// Need this for IPC functions called below
	log.Init(io.Discard, false, false)

	tun := tunnel.Desc{Name: "notrunning"}
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: &tun}
	conn, err := net.Dial("unix", getEnv(env, "BORING_SOCK"))
	if err != nil {
		t.Fatalf("could not connect to daemon")
	}
	defer conn.Close()

	if err = ipc.Write(cmd, conn); err != nil {
		t.Fatalf("%v", err.Error())
	}
	var r daemon.Resp
	if err = ipc.Read(&r, conn); err != nil {
		t.Fatalf("%v", err.Error())
	}

	if r.Success || !strings.Contains(r.Error, "tunnel not running") {
		t.Fatalf("did not get correct error message: %v", r.Error)
	}
}

// Test that Open/Close commands without a tunnel payload are rejected
// gracefully and do not crash the daemon
func TestDaemonMissingTunnel(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	log.Init(io.Discard, false, false)

	sock := getEnv(env, "BORING_SOCK")
	for _, kind := range []daemon.CmdKind{daemon.Open, daemon.Close} {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatalf("could not connect to daemon")
		}
		defer conn.Close()

		if err = ipc.Write(daemon.Cmd{Kind: kind}, conn); err != nil {
			t.Fatalf("%v", err.Error())
		}
		var r daemon.Resp
		if err = ipc.Read(&r, conn); err != nil {
			t.Fatalf("%v", err.Error())
		}

		if r.Success || !strings.Contains(r.Error, "no tunnel specified") {
			t.Fatalf("did not get correct error message: %v", r.Error)
		}
	}

	// Daemon must still be responsive
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("daemon not reachable anymore: %v", err)
	}
	defer conn.Close()

	if err = ipc.Write(daemon.Cmd{Kind: daemon.Nop}, conn); err != nil {
		t.Fatalf("%v", err.Error())
	}
	var r daemon.Resp
	if err = ipc.Read(&r, conn); err != nil {
		t.Fatalf("daemon did not respond anymore: %v", err.Error())
	}
	if !r.Success {
		t.Fatalf("expected success, got: %v", r.Error)
	}
}

// Test that the CLI will respawn a new daemon when it detects a non-matching version
func TestDaemonLaunchMismatch(t *testing.T) {
	cfg := defaultConfig
	cfg.noSpawn = false
	cfg.debug = true

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	setEnv(env, "BORING_COMMIT_OVERRIDE", "11111")

	out := testDaemonLaunch(t, env)

	if !strings.Contains(stripANSI(out), "Detected daemon build #00000 (CLI: #11111)") {
		t.Fatalf("expected incompatibility error, got: %s", out)
	}
}

// Test the output in case the daemon didn't send a version
func TestDaemonLaunchMismatch2(t *testing.T) {
	cfg := defaultConfig
	cfg.noSpawn = false
	cfg.debug = true
	cfg.commitOverride = ""

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	setEnv(env, "BORING_COMMIT_OVERRIDE", "11111")

	out := testDaemonLaunch(t, env)

	if !strings.Contains(stripANSI(out), "Detected unknown daemon build (CLI: #11111)") {
		t.Fatalf("expected incompatibility error, got: %s", out)
	}
}

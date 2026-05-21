package e2e

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// uptimeRe matches the uptime string `boring list` prints as the status of an
// open tunnel (e.g. "00m05s", "01h02m", "03d04h").
var uptimeRe = regexp.MustCompile(`^\d\d[dhm]\d\d[hms]$`)

// listStatus runs `boring list` and returns a normalized status for the row
// named name: "open" for any uptime string, otherwise the literal status word
// ("closed", "reconn", "needs auth"). It returns an empty string when the
// tunnel is not listed.
//
// The list row layout is `<status...> <name> <local> -> <remote> <via>`. The
// status is one word except "needs auth", which is two; the row is keyed off
// the "->" arrow, which is always present, so name is the field two before it.
func listStatus(t *testing.T, env []string, name string) string {
	t.Helper()
	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run list command: %v", err)
	}
	if c != 0 {
		t.Fatalf("list exit code %d: %s", c, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(stripANSI(out)), "\n") {
		fields := strings.Fields(line)
		arrow := -1
		for i, f := range fields {
			if f == "->" {
				arrow = i
				break
			}
		}
		if arrow < 2 || fields[arrow-2] != name {
			continue
		}
		status := strings.Join(fields[:arrow-2], " ")
		if uptimeRe.MatchString(status) {
			return "open"
		}
		return status
	}
	return ""
}

// waitForStatus polls `boring list` until the named tunnel reports want, or
// fails the test once the timeout elapses.
func waitForStatus(t *testing.T, env []string, name, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if listStatus(t, env, name) == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("tunnel %q did not reach status %q in time (last: %q)",
				name, want, listStatus(t, env, name))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestTunnel2FADropNeedsAuth verifies the NeedsAuth lifecycle for a dropped 2FA
// tunnel: after a keyboard-interactive tunnel's SSH connection drops, the
// daemon must keep it visible via `list` resting at "needs auth" (it cannot be
// silently auto-reconnected, since a fresh code is required), and the user must
// be able to re-open it. This is an integration test because the bug was an
// integration gap: run() set NeedsAuth correctly, but the daemon's cleanup
// goroutine unconditionally deleted the tunnel so `list` never saw the status.
func TestTunnel2FADropNeedsAuth(t *testing.T) {
	server2fa, err := startServer2FA()
	if err != nil {
		t.Fatalf("failed to start 2FA SSH server: %v", err)
	}
	defer server2fa.cleanup()

	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/config_2fa.toml"
	cfg.sshConfig = "../testdata/config/ssh_config_2fa"

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	env = setEnv(env, "BORING_AUTH_ANSWERS", test2FACode)

	// Open the 2FA tunnel.
	c, out, err := cliCommand(env, "open", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run open command: %v", err)
	}
	if c != 0 {
		t.Fatalf("open exit code %d: %s", c, out)
	}
	waitForStatus(t, env, "test-2fa", "open")

	// Drop the SSH connection. The 2FA listener stays up (closeAll only
	// severs established connections), so the tunnel can be re-opened later.
	server2fa.closeAll()

	// The tunnel is interactive, so run() does not auto-reconnect: it must
	// rest at NeedsAuth and `list` must keep reporting it.
	waitForStatus(t, env, "test-2fa", "needs auth")

	// Re-open the dropped 2FA tunnel: this must clear the needsAuth holding
	// entry and establish a fresh connection.
	c, out, err = cliCommand(env, "open", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run re-open command: %v", err)
	}
	if c != 0 {
		t.Fatalf("re-open exit code %d: %s", c, out)
	}
	if !strings.Contains(strings.ToLower(stripANSI(out)), "opened tunnel") {
		t.Fatalf("re-open did not indicate success: %s", out)
	}
	waitForStatus(t, env, "test-2fa", "open")

	// The re-opened tunnel must actually forward data.
	testTunnel(t, "localhost:49721", "localhost:49722")

	// Closing the now-running tunnel must succeed.
	c, out, err = cliCommand(env, "close", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run close command: %v", err)
	}
	if c != 0 {
		t.Fatalf("close exit code %d: %s", c, out)
	}
}

// TestTunnel2FACloseAuth verifies the other resolution of a dropped 2FA
// tunnel: instead of re-opening it, the user closes it. `close` must succeed
// against a tunnel that is only in the daemon's needsAuth holding map (its
// run() has already exited), and afterwards `list` must report it as a plain
// closed tunnel.
//
// The test name is kept short on purpose: t.TempDir() embeds it in the daemon
// Unix socket path, which macOS limits to ~104 bytes.
func TestTunnel2FACloseAuth(t *testing.T) {
	server2fa, err := startServer2FA()
	if err != nil {
		t.Fatalf("failed to start 2FA SSH server: %v", err)
	}
	defer server2fa.cleanup()

	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/config_2fa.toml"
	cfg.sshConfig = "../testdata/config/ssh_config_2fa"

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	env = setEnv(env, "BORING_AUTH_ANSWERS", test2FACode)

	c, out, err := cliCommand(env, "open", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run open command: %v", err)
	}
	if c != 0 {
		t.Fatalf("open exit code %d: %s", c, out)
	}
	waitForStatus(t, env, "test-2fa", "open")

	server2fa.closeAll()
	waitForStatus(t, env, "test-2fa", "needs auth")

	// Closing a NeedsAuth tunnel must succeed.
	c, out, err = cliCommand(env, "close", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run close command: %v", err)
	}
	if c != 0 {
		t.Fatalf("close exit code %d: %s", c, out)
	}

	// Once closed, the tunnel no longer rests at NeedsAuth.
	if s := listStatus(t, env, "test-2fa"); s != "closed" {
		t.Fatalf("tunnel should be closed after closing from NeedsAuth, got %q", s)
	}
}

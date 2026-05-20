package e2e

import (
	"strings"
	"testing"
)

// testCmdConfig returns the e2e config pointing at the dedicated `boring test`
// fixture: a reachable tunnel against the in-process public-key SSH server and
// an unreachable one with nothing listening.
func testCmdConfig() config {
	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/config_test_cmd.toml"
	return cfg
}

// TestTestCommandReachable checks that `boring test` reports a reachable
// tunnel as a successful connection and exits 0. The command runs the SSH
// handshake in the CLI process itself, so no daemon is needed.
func TestTestCommandReachable(t *testing.T) {
	env, err := makeEnv(testCmdConfig(), t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "test", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "connection OK") {
		t.Fatalf("output did not indicate success: %s", out)
	}
}

// TestTestCommandMultiForward checks that `boring test` reports a reachable
// multi-forward tunnel as a successful connection and exits 0. The command
// verifies only the SSH handshake and auth, opening no listeners, so it must
// succeed regardless of how many [[tunnels.forward]] blocks the tunnel carries.
func TestTestCommandMultiForward(t *testing.T) {
	env, err := makeEnv(testCmdConfig(), t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "test", "test-multi")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "connection OK") {
		t.Fatalf("output did not indicate success: %s", out)
	}
}

// TestTestCommandUnreachable checks that `boring test` reports an unreachable
// tunnel as a failed connection and exits 1.
func TestTestCommandUnreachable(t *testing.T) {
	env, err := makeEnv(testCmdConfig(), t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "test", "test-unreachable")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "connection failed") {
		t.Fatalf("output did not indicate failure: %s", out)
	}
}

// TestTestCommandNoMatch checks that `boring test` exits 1 when no tunnel
// matches the given pattern.
func TestTestCommandNoMatch(t *testing.T) {
	env, err := makeEnv(testCmdConfig(), t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "test", "doesnotexist")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "No tunnels match pattern") {
		t.Fatalf("output did not indicate no match: %s", out)
	}
}

// TestTestCommandNoPattern checks that `boring test` with no pattern argument
// exits 1 with a usage error.
func TestTestCommandNoPattern(t *testing.T) {
	env, err := makeEnv(testCmdConfig(), t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "requires at least one 'pattern' argument") {
		t.Fatalf("output did not indicate missing argument: %s", out)
	}
}

// TestTestCommand2FA proves `boring test` drives interactive (keyboard-
// interactive / 2FA) authentication: with the correct code supplied through
// the BORING_AUTH_ANSWERS test hook, the test-2fa tunnel connects OK.
func TestTestCommand2FA(t *testing.T) {
	server2fa, err := startServer2FA()
	if err != nil {
		t.Fatalf("failed to start 2FA SSH server: %v", err)
	}
	defer server2fa.cleanup()

	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/config_2fa.toml"
	cfg.sshConfig = "../testdata/config/ssh_config_2fa"

	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	env = setEnv(env, "BORING_AUTH_ANSWERS", test2FACode)

	c, out, err := cliCommand(env, "test", "test-2fa")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "connection OK") {
		t.Fatalf("output did not indicate success: %s", out)
	}
}

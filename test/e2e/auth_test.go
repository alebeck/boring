package e2e

import (
	"strings"
	"testing"
)

// TestOpen2FA exercises the full keyboard-interactive (2FA) authentication
// path end-to-end: `boring open` on a tunnel whose target is the in-process
// 2FA SSH server, through the real daemon. It verifies that the correct code
// authenticates and forwards data, and that a wrong code is rejected.
func TestOpen2FA(t *testing.T) {
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

	t.Run("correct code authenticates and forwards data", func(t *testing.T) {
		okEnv := setEnv(append([]string(nil), env...), "BORING_AUTH_ANSWERS", test2FACode)

		c, out, err := cliCommand(okEnv, "open", "test-2fa")
		if err != nil {
			t.Fatalf("failed to run CLI command: %v", err)
		}
		if c != 0 {
			t.Fatalf("exit code %d, should be 0: %s", c, out)
		}
		if !strings.Contains(strings.ToLower(stripANSI(out)), "opened tunnel") {
			t.Fatalf("output did not indicate success: %s", out)
		}

		// The tunnel must actually forward data once open.
		testTunnel(t, "localhost:49721", "localhost:49722")

		// Close the tunnel so the wrong-code subtest opens it fresh.
		c, out, err = cliCommand(env, "close", "test-2fa")
		if err != nil {
			t.Fatalf("failed to run CLI command: %v", err)
		}
		if c != 0 {
			t.Fatalf("exit code %d, should be 0: %s", c, out)
		}
	})

	t.Run("wrong code is rejected", func(t *testing.T) {
		badEnv := setEnv(append([]string(nil), env...), "BORING_AUTH_ANSWERS", "000000")

		c, out, err := cliCommand(badEnv, "open", "test-2fa")
		if err != nil {
			t.Fatalf("failed to run CLI command: %v", err)
		}
		if c == 0 {
			t.Fatalf("exit code %d, should be non-zero for a wrong 2FA code: %s", c, out)
		}
		if !strings.Contains(strings.ToLower(stripANSI(out)), "could not open tunnel") {
			t.Fatalf("output did not indicate authentication failure: %s", out)
		}
	})
}

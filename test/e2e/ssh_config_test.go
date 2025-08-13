package e2e

import (
	"strings"
	"testing"
)

// Test that some simple "%" substitutions are working.
// These are handled by boring, not ssh_config.
func TestSubstitutions(t *testing.T) {
	cfg := defaultConfig
	cfg.sshConfig = "../testdata/config/ssh_config_subst"
	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(strings.ToLower(out), "opened tunnel") {
		t.Errorf("output did not indicate success: %s", out)
	}
}

func TestRSA(t *testing.T) {
	// TODO
}

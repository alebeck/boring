package e2e

import (
	"testing"
)

func TestAgent(t *testing.T) {
	cfg := defaultConfig
	cfg.sshConfig = "../testdata/config/ssh_config_no_id"
	cfg.useAgent = true
	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	// Start up agent
	cancel, err = startAgent(getEnv(env, "SSH_AUTH_SOCK"))
	if err != nil {
		t.Fatalf("could not start agent: %v", err)
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
}

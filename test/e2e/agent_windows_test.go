package e2e

import "testing"

// Agents on Windows usually listen on a named pipe
func TestAgentReconnectPipe(t *testing.T) {
	cfg := defaultConfig
	cfg.sshConfig = "../testdata/config/ssh_config_no_id"
	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	env = append(env, `SSH_AUTH_SOCK=\\.\pipe\boring-test`)
	cancel, err := daemonWithCancel(env)
	if err != nil {
		t.Fatalf("could not start daemon: %v", err)
	}
	defer cancel()
	testAgentReconnect(t, env)
}

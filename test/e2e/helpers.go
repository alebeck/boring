package e2e

import (
	"context"
	"fmt"
	"github.com/alebeck/boring/internal/daemon"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	binary      = "../../boring.test"
	cliTimeout  = 10 * time.Second
	connTimeout = 5 * time.Second
)

var testMsg = []byte("hello through tunnel")

type config struct {
	boringConfig string
	sshConfig    string
	noSpawn      bool
	debug        bool
	useAgent     bool
}

var defaultConfig = config{
	boringConfig: "../testdata/config/config.toml",
	sshConfig:    "../testdata/config/ssh_config",
	noSpawn:      true,
	debug:        false,
	useAgent:     false,
}

func makeEnv(c config, t *testing.T) ([]string, error) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "boringd.log")
	sockFile := filepath.Join(tmpDir, "boringd.sock")
	// fmt.Printf("daemon logs at: %s\n", logFile)
	// fmt.Printf("daemon socket at: %s\n", sockFile)

	env := append(
		os.Environ(),
		"BORING_CONFIG="+c.boringConfig,
		"BORING_LOG_FILE="+logFile,
		"BORING_SOCK="+sockFile,
		"BORING_FORCE_INTERACTIVE=1",
		"BORING_SSH_CONFIG="+c.sshConfig,
	)

	if c.noSpawn {
		env = append(env, "BORING_NO_SPAWN=1")
	}

	if c.debug {
		env = append(env, "DEBUG=1")
	}

	if c.useAgent {
		env = append(env, "SSH_AUTH_SOCK="+filepath.Join(tmpDir, "agent.sock"))
	} else {
		env = append(env, "SSH_AUTH_SOCK=doesnotexist")
	}

	return env, nil
}

func makeDefaultEnv(t *testing.T) ([]string, error) {
	return makeEnv(defaultConfig, t)
}

func getEnv(env []string, key string) (val string) {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			val = strings.Split(e, "=")[1]
		}
	}
	return
}

func daemonWithCancel(env []string) (context.CancelFunc, error) {
	//ctx, cancel := context.WithCancel(context.Background())
	// cmd := exec.CommandContext(ctx, binary, daemon.Flag)
	cmd := exec.Command(binary, daemon.Flag)
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		// cancel()
		return nil, err
	}
	// Give the daemon some time to start
	time.Sleep(100 * time.Millisecond)

	cancel := func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
	}
	return cancel, nil
}

func makeEnvWithDaemon(c config, t *testing.T) ([]string, context.CancelFunc, error) {
	env, err := makeEnv(c, t)
	if err != nil {
		return nil, nil, err
	}
	cancel, err := daemonWithCancel(env)
	if err != nil {
		return nil, nil, fmt.Errorf("could not start daemon: %w", err)
	}
	return env, cancel, nil
}

func makeDefaultEnvWithDaemon(t *testing.T) ([]string, context.CancelFunc, error) {
	return makeEnvWithDaemon(defaultConfig, t)
}

func cliCommand(env []string, cmds ...string) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, cmds...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(output), nil
	}
	if err != nil {
		// some other error occurred while running the command
		return 0, "", err
	}
	return 0, string(output), nil
}

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

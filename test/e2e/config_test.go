package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigCreate(t *testing.T) {
	cfg := defaultConfig
	cfg.boringConfig = t.TempDir() + "/config.toml" // not existing

	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	_, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	// Return code will be 1 because daemon is not running

	if !strings.Contains(out, "Hi! Created boring config file") {
		t.Errorf("output did not indicate creating a config file: %s", out)
	}
	if _, err := os.Stat(cfg.boringConfig); os.IsNotExist(err) {
		t.Fatalf("expected file %q to exist, but it does not", cfg.boringConfig)
	} else if err != nil {
		t.Fatalf("error checking file %q: %v", cfg.boringConfig, err)
	}
}

func TestConfigCreateMkdirFails(t *testing.T) {
	cfg := defaultConfig
	// Place a regular file where ensureConfig will try to MkdirAll a parent dir.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}
	cfg.boringConfig = filepath.Join(blocker, "config.toml")

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, expected 1", c)
	}
	if !strings.Contains(out, "could not create config file") {
		t.Errorf("output did not indicate config creation failure: %s", out)
	}
}

func TestConfigCreateOpenFails(t *testing.T) {
	cfg := defaultConfig
	// Pre-create a read-only parent directory so OpenFile with O_CREATE fails
	// while MkdirAll on the existing dir succeeds.
	parent := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(parent, 0500); err != nil {
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0700) })
	cfg.boringConfig = filepath.Join(parent, "config.toml")

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, expected 1", c)
	}
	if !strings.Contains(out, "could not create config file") {
		t.Errorf("output did not indicate config creation failure: %s", out)
	}
}

func TestEdit(t *testing.T) {
	cfg := defaultConfig
	env, err := makeEnv(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	tmpEditor := t.TempDir() + "/boring-editor.sh"
	if err := os.WriteFile(tmpEditor,
		[]byte("#!/bin/sh\necho $1"), 0755); err != nil {
		t.Fatalf("failed to create temporary editor script: %v", err)
	}
	env = append(env, "EDITOR="+tmpEditor)

	c, out, err := cliCommand(env, "edit")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(out, cfg.boringConfig) {
		t.Errorf("editor script did not emit config file path: %s", out)
	}
}

func testInvalidConfig(t *testing.T, cfgPath string) {
	cfg := defaultConfig
	cfg.boringConfig = cfgPath
	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, expected 1", c)
	}
	if !(strings.Contains(out, "tunnel names cannot be empty, ") ||
		strings.Contains(out, "found duplicated tunnel name")) {
		t.Errorf("output did not indicate invalid tunnel name: %s", out)
	}
}

func TestInvalidSpace(t *testing.T) {
	testInvalidConfig(t, "../testdata/config/invalid/contains_space.toml")
}

func TestInvalidEmpty(t *testing.T) {
	testInvalidConfig(t, "../testdata/config/invalid/empty_name.toml")
}

func TestInvalidSpecial(t *testing.T) {
	testInvalidConfig(t, "../testdata/config/invalid/special_prefix.toml")
}

func TestInvalidGlob(t *testing.T) {
	testInvalidConfig(t, "../testdata/config/invalid/contains_glob.toml")
}

func TestDoubleName(t *testing.T) {
	testInvalidConfig(t, "../testdata/config/invalid/double_name.toml")
}

func TestInvalidGroup(t *testing.T) {
	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/invalid/invalid_group.toml"
	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, expected 1", c)
	}
	if !strings.Contains(out, "groups cannot") {
		t.Errorf("output did not indicate invalid group name: %s", out)
	}
}

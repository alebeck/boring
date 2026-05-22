package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	config_pkg "github.com/alebeck/boring/internal/config"
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
	if !strings.Contains(out, "group names cannot") {
		t.Errorf("output did not indicate invalid group name: %s", out)
	}
}

func TestEnvVarExpansionInHost(t *testing.T) {
	// Create a config file with environment variable references in the host field
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "envtest"
host = "${BORING_TEST_HOST}"
local = 49711
remote = "localhost:49712"

[[tunnels]]
name = "envtest2"
host = "${BORING_TEST_USER}@${BORING_TEST_HOST}"
local = 49713
remote = "localhost:49714"

[[tunnels]]
name = "noenv"
host = "static-host.example.com"
local = 49715
remote = "localhost:49716"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set environment variables
	t.Setenv("BORING_TEST_HOST", "myserver.example.com")
	t.Setenv("BORING_TEST_USER", "testuser")

	// Override the config path and load
	t.Setenv("BORING_CONFIG", cfgPath)

	// We need to reload the config path since init() already ran
	origPath := config_pkg.Path
	config_pkg.Path = cfgPath
	defer func() { config_pkg.Path = origPath }()

	cfg, err := config_pkg.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	tests := []struct {
		name         string
		expectedHost string
	}{
		{"envtest", "myserver.example.com"},
		{"envtest2", "testuser@myserver.example.com"},
		{"noenv", "static-host.example.com"},
	}

	for _, tt := range tests {
		desc, ok := cfg.TunnelsMap[tt.name]
		if !ok {
			t.Errorf("tunnel %q not found in config", tt.name)
			continue
		}
		if desc.Host != tt.expectedHost {
			t.Errorf("tunnel %q: expected host %q, got %q", tt.name, tt.expectedHost, desc.Host)
		}
	}
}

func TestEnvVarExpansionUnsetVar(t *testing.T) {
	// When an env var is not set, os.Expand replaces it with empty string
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "unsetenv"
host = "${BORING_UNSET_VAR_12345}"
local = 49711
remote = "localhost:49712"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Make sure the variable is not set
	os.Unsetenv("BORING_UNSET_VAR_12345")

	origPath := config_pkg.Path
	config_pkg.Path = cfgPath
	defer func() { config_pkg.Path = origPath }()

	cfg, err := config_pkg.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	desc, ok := cfg.TunnelsMap["unsetenv"]
	if !ok {
		t.Fatal("tunnel 'unsetenv' not found")
	}
	if desc.Host != "" {
		t.Errorf("expected empty host for unset env var, got %q", desc.Host)
	}
}

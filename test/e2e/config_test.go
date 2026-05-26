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

func TestEnvVarExpansionWithDefault(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "withdefault"
host = "${BORING_TEST_DEFHOST:-fallback.example.com}"
local = 49711
remote = "localhost:49712"

[[tunnels]]
name = "overridden"
host = "${BORING_TEST_DEFHOST2:-fallback2.example.com}"
local = 49713
remote = "localhost:49714"

[[tunnels]]
name = "multidefault"
host = "${BORING_TEST_DEFUSER:-admin}@${BORING_TEST_DEFHOST3:-default-host.local}"
local = 49715
remote = "localhost:49716"

[[tunnels]]
name = "emptydefault"
host = "${BORING_TEST_UNSET_XXX:-}"
local = 49717
remote = "localhost:49718"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// BORING_TEST_DEFHOST is NOT set, so default should be used
	os.Unsetenv("BORING_TEST_DEFHOST")

	// BORING_TEST_DEFHOST2 IS set, so it should override the default
	t.Setenv("BORING_TEST_DEFHOST2", "real-host.example.com")

	// Neither BORING_TEST_DEFUSER nor BORING_TEST_DEFHOST3 are set
	os.Unsetenv("BORING_TEST_DEFUSER")
	os.Unsetenv("BORING_TEST_DEFHOST3")

	// BORING_TEST_UNSET_XXX is not set, default is empty string
	os.Unsetenv("BORING_TEST_UNSET_XXX")

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
		{"withdefault", "fallback.example.com"},
		{"overridden", "real-host.example.com"},
		{"multidefault", "admin@default-host.local"},
		{"emptydefault", ""},
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

func TestEnvVarExpansionAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "${BORING_TEST_NAME}"
host = "${BORING_TEST_HOST}"
local = "${BORING_TEST_LOCAL}"
remote = "${BORING_TEST_REMOTE}"
user = "${BORING_TEST_USER}"
identity = "${BORING_TEST_IDENTITY}"
group = "${BORING_TEST_GROUP}"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set all environment variables
	t.Setenv("BORING_TEST_NAME", "mytunnel")
	t.Setenv("BORING_TEST_HOST", "myhost.example.com")
	t.Setenv("BORING_TEST_LOCAL", "localhost:5432")
	t.Setenv("BORING_TEST_REMOTE", "dbserver:5432")
	t.Setenv("BORING_TEST_USER", "dbuser")
	t.Setenv("BORING_TEST_IDENTITY", "/home/user/.ssh/id_rsa")
	t.Setenv("BORING_TEST_GROUP", "databases")

	origPath := config_pkg.Path
	config_pkg.Path = cfgPath
	defer func() { config_pkg.Path = origPath }()

	cfg, err := config_pkg.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	desc, ok := cfg.TunnelsMap["mytunnel"]
	if !ok {
		t.Fatal("tunnel 'mytunnel' not found in config")
	}

	if desc.Name != "mytunnel" {
		t.Errorf("Name: expected %q, got %q", "mytunnel", desc.Name)
	}
	if desc.Host != "myhost.example.com" {
		t.Errorf("Host: expected %q, got %q", "myhost.example.com", desc.Host)
	}
	if desc.LocalAddress.String() != "localhost:5432" {
		t.Errorf("LocalAddress: expected %q, got %q", "localhost:5432", desc.LocalAddress.String())
	}
	if desc.RemoteAddress.String() != "dbserver:5432" {
		t.Errorf("RemoteAddress: expected %q, got %q", "dbserver:5432", desc.RemoteAddress.String())
	}
	if desc.User != "dbuser" {
		t.Errorf("User: expected %q, got %q", "dbuser", desc.User)
	}
	if desc.IdentityFile != "/home/user/.ssh/id_rsa" {
		t.Errorf("IdentityFile: expected %q, got %q", "/home/user/.ssh/id_rsa", desc.IdentityFile)
	}
	if desc.Group != "databases" {
		t.Errorf("Group: expected %q, got %q", "databases", desc.Group)
	}
}

func TestEnvVarExpansionAllFieldsWithDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "${BORING_TEST_NAME2:-defaulttunnel}"
host = "${BORING_TEST_HOST2:-default.example.com}"
local = "${BORING_TEST_LOCAL2:-8080}"
remote = "${BORING_TEST_REMOTE2:-localhost:80}"
user = "${BORING_TEST_USER2:-defaultuser}"
identity = "${BORING_TEST_IDENTITY2:-~/.ssh/id_ed25519}"
group = "${BORING_TEST_GROUP2:-defaultgroup}"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Unset all variables so defaults are used
	os.Unsetenv("BORING_TEST_NAME2")
	os.Unsetenv("BORING_TEST_HOST2")
	os.Unsetenv("BORING_TEST_LOCAL2")
	os.Unsetenv("BORING_TEST_REMOTE2")
	os.Unsetenv("BORING_TEST_USER2")
	os.Unsetenv("BORING_TEST_IDENTITY2")
	os.Unsetenv("BORING_TEST_GROUP2")

	origPath := config_pkg.Path
	config_pkg.Path = cfgPath
	defer func() { config_pkg.Path = origPath }()

	cfg, err := config_pkg.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	desc, ok := cfg.TunnelsMap["defaulttunnel"]
	if !ok {
		t.Fatal("tunnel 'defaulttunnel' not found in config")
	}

	if desc.Name != "defaulttunnel" {
		t.Errorf("Name: expected %q, got %q", "defaulttunnel", desc.Name)
	}
	if desc.Host != "default.example.com" {
		t.Errorf("Host: expected %q, got %q", "default.example.com", desc.Host)
	}
	if desc.LocalAddress.String() != "8080" {
		t.Errorf("LocalAddress: expected %q, got %q", "8080", desc.LocalAddress.String())
	}
	if desc.RemoteAddress.String() != "localhost:80" {
		t.Errorf("RemoteAddress: expected %q, got %q", "localhost:80", desc.RemoteAddress.String())
	}
	if desc.User != "defaultuser" {
		t.Errorf("User: expected %q, got %q", "defaultuser", desc.User)
	}
	if desc.IdentityFile != "~/.ssh/id_ed25519" {
		t.Errorf("IdentityFile: expected %q, got %q", "~/.ssh/id_ed25519", desc.IdentityFile)
	}
	if desc.Group != "defaultgroup" {
		t.Errorf("Group: expected %q, got %q", "defaultgroup", desc.Group)
	}
}

func TestEnvVarExpansionMixedFields(t *testing.T) {
	// Test a mix of env vars set and defaults used
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")

	cfgContent := `[[tunnels]]
name = "mixedtest"
host = "${BORING_MIX_HOST:-fallback.local}"
local = "${BORING_MIX_LOCAL:-9999}"
remote = "${BORING_MIX_REMOTE}"
user = "${BORING_MIX_USER:-fallbackuser}"
identity = "${BORING_MIX_IDENTITY}"
group = "${BORING_MIX_GROUP:-fallbackgroup}"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set some variables, leave others unset
	t.Setenv("BORING_MIX_HOST", "realhost.example.com") // overrides default
	os.Unsetenv("BORING_MIX_LOCAL")                     // uses default
	t.Setenv("BORING_MIX_REMOTE", "realremote:443")     // no default, set
	os.Unsetenv("BORING_MIX_USER")                      // uses default
	os.Unsetenv("BORING_MIX_IDENTITY")                  // no default, empty
	t.Setenv("BORING_MIX_GROUP", "realgroup")           // overrides default

	origPath := config_pkg.Path
	config_pkg.Path = cfgPath
	defer func() { config_pkg.Path = origPath }()

	cfg, err := config_pkg.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	desc, ok := cfg.TunnelsMap["mixedtest"]
	if !ok {
		t.Fatal("tunnel 'mixedtest' not found in config")
	}

	if desc.Host != "realhost.example.com" {
		t.Errorf("Host: expected %q, got %q", "realhost.example.com", desc.Host)
	}
	if desc.LocalAddress.String() != "9999" {
		t.Errorf("LocalAddress: expected %q, got %q", "9999", desc.LocalAddress.String())
	}
	if desc.RemoteAddress.String() != "realremote:443" {
		t.Errorf("RemoteAddress: expected %q, got %q", "realremote:443", desc.RemoteAddress.String())
	}
	if desc.User != "fallbackuser" {
		t.Errorf("User: expected %q, got %q", "fallbackuser", desc.User)
	}
	if desc.IdentityFile != "" {
		t.Errorf("IdentityFile: expected empty string, got %q", desc.IdentityFile)
	}
	if desc.Group != "realgroup" {
		t.Errorf("Group: expected %q, got %q", "realgroup", desc.Group)
	}
}

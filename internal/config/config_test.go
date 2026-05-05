package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadVPNConfig(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[vpn]
poll_interval = 5
stable_for = 15
cidrs = ["10.0.0.0/8"]

[[tunnels]]
name = "internal-api"
local = "9000"
remote = "localhost:9000"
host = "jumpbox"
vpn_required = true
auto_open_when_vpn = true
auto_close_when_vpn_lost = true
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.VPN.PollInterval != 5 {
		t.Fatalf("VPN.PollInterval = %d, want 5", cfg.VPN.PollInterval)
	}
	if cfg.VPN.StableFor != 15 {
		t.Fatalf("VPN.StableFor = %d, want 15", cfg.VPN.StableFor)
	}
	if !cfg.VPN.Enabled() {
		t.Fatal("VPN.Enabled() = false, want true")
	}
	if len(cfg.VPN.ParsedCIDRs) != 1 {
		t.Fatalf("len(VPN.ParsedCIDRs) = %d, want 1", len(cfg.VPN.ParsedCIDRs))
	}

	desc := cfg.TunnelsMap["internal-api"]
	if desc == nil {
		t.Fatal("internal-api missing from TunnelsMap")
	}
	if !desc.VPNRequired || !desc.AutoOpenWhenVPN || !desc.AutoCloseWhenVPNLost {
		t.Fatalf("VPN tunnel flags were not parsed: %+v", desc)
	}
}

func TestLoadVPNGroupConfig(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[group.work]
vpn_required = true
auto_open_when_vpn = true
auto_close_when_vpn_lost = true

[[tunnels]]
name = "internal-api"
group = "work"
local = "9000"
remote = "localhost:9000"
host = "jumpbox"
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	desc := cfg.TunnelsMap["internal-api"]
	if desc == nil {
		t.Fatal("internal-api missing from TunnelsMap")
	}
	if !desc.VPNRequired || !desc.AutoOpenWhenVPN || !desc.AutoCloseWhenVPNLost {
		t.Fatalf("VPN group flags were not applied: %+v", desc)
	}
}

func TestLoadVPNDefaultGroupConfig(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[group.default]
vpn_required = true
auto_open_when_vpn = true

[[tunnels]]
name = "internal-api"
local = "9000"
remote = "localhost:9000"
host = "jumpbox"
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	desc := cfg.TunnelsMap["internal-api"]
	if desc == nil {
		t.Fatal("internal-api missing from TunnelsMap")
	}
	if !desc.VPNRequired || !desc.AutoOpenWhenVPN {
		t.Fatalf("VPN default group flags were not applied: %+v", desc)
	}
}

func TestLoadVPNInvalidGroupConfig(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[group."bad group"]
vpn_required = true
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "group names cannot") {
		t.Fatalf("Load() error = %q, want invalid group", err)
	}
}

func TestLoadVPNInvalidCIDR(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[vpn]
cidrs = ["not-a-cidr"]
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid VPN CIDR") {
		t.Fatalf("Load() error = %q, want invalid VPN CIDR", err)
	}
}

func TestLoadVPNDefaults(t *testing.T) {
	oldPath := Path
	t.Cleanup(func() { Path = oldPath })

	Path = filepath.Join(t.TempDir(), "config.toml")
	content := `
[vpn]
cidrs = ["10.0.0.0/8"]
`
	if err := os.WriteFile(Path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.VPN.PollInterval != defaultVPNPollInterval {
		t.Fatalf("VPN.PollInterval = %d, want %d", cfg.VPN.PollInterval, defaultVPNPollInterval)
	}
	if cfg.VPN.StableFor != defaultVPNStableFor {
		t.Fatalf("VPN.StableFor = %d, want %d", cfg.VPN.StableFor, defaultVPNStableFor)
	}
}

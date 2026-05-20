package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alebeck/boring/internal/tunnel"
)

// writeConfig writes content to a temporary .boring.toml file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".boring.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestLoadLegacySingleForward(t *testing.T) {
	path := writeConfig(t, `
[[tunnels]]
name   = "dev"
local  = "9000"
remote = "localhost:9000"
host   = "devhost"
mode   = "local"
`)
	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(cfg.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(cfg.Tunnels))
	}
	d := cfg.Tunnels[0]

	// Legacy fields stay populated, unchanged.
	if d.LocalAddress != "9000" || d.RemoteAddress != "localhost:9000" {
		t.Fatalf("legacy fields altered: local=%q remote=%q", d.LocalAddress, d.RemoteAddress)
	}
	if d.Mode != tunnel.Local {
		t.Fatalf("legacy mode altered: got %v", d.Mode)
	}

	// Forwards is folded from the legacy shorthand: exactly one forward.
	if len(d.Forwards) != 1 {
		t.Fatalf("expected 1 forward, got %d: %+v", len(d.Forwards), d.Forwards)
	}
	f := d.Forwards[0]
	if f.LocalAddress != "9000" {
		t.Errorf("forward local = %q, want \"9000\"", f.LocalAddress)
	}
	if f.RemoteAddress != "localhost:9000" {
		t.Errorf("forward remote = %q, want \"localhost:9000\"", f.RemoteAddress)
	}
	if f.Mode != tunnel.Local {
		t.Errorf("forward mode = %v, want Local", f.Mode)
	}
	if f.Name != "" {
		t.Errorf("implicit forward should have no name, got %q", f.Name)
	}
}

func TestLoadMultipleForwards(t *testing.T) {
	path := writeConfig(t, `
[[tunnels]]
name = "prod"
host = "bastion"
user = "deploy"

  [[tunnels.forward]]
  name   = "db"
  local  = "5432"
  remote = "db.internal:5432"

  [[tunnels.forward]]
  name   = "cache"
  local  = "6379"
  remote = "redis.internal:6379"
  mode   = "local"
`)
	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(cfg.Tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(cfg.Tunnels))
	}
	d := cfg.Tunnels[0]
	if len(d.Forwards) != 2 {
		t.Fatalf("expected 2 forwards, got %d: %+v", len(d.Forwards), d.Forwards)
	}

	want := []tunnel.Forward{
		{Name: "db", LocalAddress: "5432", RemoteAddress: "db.internal:5432", Mode: tunnel.Local},
		{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis.internal:6379", Mode: tunnel.Local},
	}
	for i, w := range want {
		got := d.Forwards[i]
		if got != w {
			t.Errorf("Forwards[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestLoadForwardsNeverEmpty(t *testing.T) {
	// A legacy socks tunnel folds its shorthand into a single implicit forward.
	path := writeConfig(t, `
[[tunnels]]
name  = "socks"
host  = "vps"
local = "1080"
mode  = "socks"
`)
	cfg, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	d := cfg.Tunnels[0]
	if len(d.Forwards) != 1 {
		t.Fatalf("expected 1 forward, got %d", len(d.Forwards))
	}
	if d.Forwards[0].Mode != tunnel.Socks {
		t.Errorf("implicit forward mode = %v, want Socks", d.Forwards[0].Mode)
	}
}

// TestLoadForwardsValidation exercises the multi-forward validation rules
// enforced by config.Load: both-set, no-forward, per-forward required
// addresses, and forward-name uniqueness.
func TestLoadForwardsValidation(t *testing.T) {
	cases := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "valid multi-forward",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  name   = "db"
  local  = "5432"
  remote = "db.internal:5432"

  [[tunnels.forward]]
  name   = "cache"
  local  = "6379"
  remote = "redis.internal:6379"
`,
			wantErr: false,
		},
		{
			name: "valid single-forward block",
			config: `
[[tunnels]]
name = "web"
host = "vps"

  [[tunnels.forward]]
  local  = "8080"
  remote = "localhost:8080"
`,
			wantErr: false,
		},
		{
			name: "both legacy shorthand and forward blocks",
			config: `
[[tunnels]]
name   = "mixed"
host   = "bastion"
local  = "9000"
remote = "localhost:9000"

  [[tunnels.forward]]
  local  = "5432"
  remote = "db.internal:5432"
`,
			wantErr: true,
		},
		{
			name: "no forward defined",
			config: `
[[tunnels]]
name = "empty"
host = "bastion"
`,
			wantErr: true,
		},
		{
			name: "local-mode forward missing local address",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  remote = "db.internal:5432"
  mode   = "local"
`,
			wantErr: true,
		},
		{
			name: "remote-mode forward missing remote address",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  local = "8080"
  mode  = "remote"
`,
			wantErr: true,
		},
		{
			name: "remote-mode forward missing local address",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  remote = "8080"
  mode   = "remote"
`,
			wantErr: true,
		},
		{
			name: "duplicate forward name",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  name   = "svc"
  local  = "5432"
  remote = "db.internal:5432"

  [[tunnels.forward]]
  name   = "svc"
  local  = "6379"
  remote = "redis.internal:6379"
`,
			wantErr: true,
		},
		{
			name: "two unnamed forwards are allowed",
			config: `
[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  local  = "5432"
  remote = "db.internal:5432"

  [[tunnels.forward]]
  local  = "6379"
  remote = "redis.internal:6379"
`,
			wantErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeConfig(t, c.config)
			_, err := loadFrom(path)
			if (err != nil) != c.wantErr {
				t.Fatalf("loadFrom() error = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

// TestLoadLegacyConfigsStillLoad confirms existing single-forward and legacy
// configs keep loading unchanged after the multi-forward validation was added.
func TestLoadLegacyConfigsStillLoad(t *testing.T) {
	cases := []string{
		`
[[tunnels]]
name   = "dev"
host   = "devhost"
local  = "9000"
remote = "localhost:9000"
`,
		`
[[tunnels]]
name   = "rev"
host   = "vps"
local  = "localhost:8080"
remote = "8080"
mode   = "remote"
`,
		`
[[tunnels]]
name  = "socks"
host  = "vps"
local = "1080"
mode  = "socks"
`,
		`
[[tunnels]]
name   = "rsocks"
host   = "vps"
remote = "1080"
mode   = "socks-remote"
`,
	}
	for i, cfg := range cases {
		t.Run(cfg, func(t *testing.T) {
			path := writeConfig(t, cfg)
			if _, err := loadFrom(path); err != nil {
				t.Fatalf("case %d: loadFrom: %v", i, err)
			}
		})
	}
}

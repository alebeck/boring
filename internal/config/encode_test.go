package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/tunnel"
)

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".boring.toml")
	cfg := &Config{Tunnels: []tunnel.Desc{
		{Name: "dev", Host: "dev-server", Mode: tunnel.Local,
			LocalAddress: "9000", RemoteAddress: "localhost:9000"},
	}}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("saving a fresh config must not create a .bak file; stat err = %v", err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(got.Tunnels) != 1 || got.Tunnels[0].Name != "dev" {
		t.Fatalf("round trip lost data: %+v", got.Tunnels)
	}
	if got.Tunnels[0].Mode != tunnel.Local || string(got.Tunnels[0].LocalAddress) != "9000" {
		t.Fatalf("round trip altered fields: %+v", got.Tunnels[0])
	}
}

func TestSaveBackupOnceOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".boring.toml")
	if err := os.WriteFile(path, []byte("# original hand-written\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if !strings.Contains(string(bak), "original hand-written") {
		t.Fatalf("first save did not back up the original; .bak = %q", bak)
	}
	// Second save must not overwrite the pristine .bak.
	if err := Save(cfg, path); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	bak2, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("reading backup after second save: %v", err)
	}
	if string(bak) != string(bak2) {
		t.Fatal("second save overwrote the .bak backup")
	}
}

func TestSaveInvalidConfigRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".boring.toml")
	cfg := &Config{Tunnels: []tunnel.Desc{{Name: ""}}} // empty name is invalid
	if err := Save(cfg, path); err == nil {
		t.Fatal("expected Save to reject an invalid config")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("invalid config must not be written to disk")
	}
}

// TestSaveSingleForwardRoundTrip checks that a legacy single-forward tunnel
// loads, saves, and reloads with its Forwards preserved, and that the saved
// file uses the legacy local/remote shorthand rather than a [[tunnels.forward]]
// block (a single-forward config must not be churned into array syntax).
func TestSaveSingleForwardRoundTrip(t *testing.T) {
	src := writeConfig(t, `
[[tunnels]]
name   = "dev"
host   = "devhost"
local  = "9000"
remote = "localhost:9000"
`)
	loaded, err := loadFrom(src)
	if err != nil {
		t.Fatalf("initial loadFrom: %v", err)
	}

	out := filepath.Join(t.TempDir(), ".boring.toml")
	if err := Save(loaded, out); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `local = "9000"`) ||
		!strings.Contains(text, `remote = "localhost:9000"`) {
		t.Fatalf("saved config lost the legacy shorthand:\n%s", text)
	}
	if strings.Contains(text, "[[tunnels.forward]]") {
		t.Fatalf("single-forward tunnel must not be written as a forward block:\n%s", text)
	}

	reloaded, err := loadFrom(out)
	if err != nil {
		t.Fatalf("reload after Save: %v", err)
	}
	assertForwardsEqual(t, loaded.Tunnels, reloaded.Tunnels)
}

// TestSaveMultiForwardRoundTrip checks that a multi-forward tunnel loads,
// saves, and reloads with its Forwards preserved, and that the saved file uses
// [[tunnels.forward]] blocks with no tunnel-level local/remote shorthand.
func TestSaveMultiForwardRoundTrip(t *testing.T) {
	src := writeConfig(t, `
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
	loaded, err := loadFrom(src)
	if err != nil {
		t.Fatalf("initial loadFrom: %v", err)
	}

	out := filepath.Join(t.TempDir(), ".boring.toml")
	if err := Save(loaded, out); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "[[tunnels.forward]]") {
		t.Fatalf("multi-forward tunnel must be written as forward blocks:\n%s", text)
	}
	// No tunnel-level shorthand: the only local/remote keys must appear after
	// the first [[tunnels.forward]] header. A tunnel-level local/remote would
	// make the file fail to reload (config.Load rejects shorthand + blocks).
	if hasShorthandBeforeForwardBlock(text) {
		t.Fatalf("multi-forward tunnel must not write tunnel-level local/remote:\n%s", text)
	}

	reloaded, err := loadFrom(out)
	if err != nil {
		t.Fatalf("reload after Save: %v", err)
	}
	assertForwardsEqual(t, loaded.Tunnels, reloaded.Tunnels)
}

// TestSaveMixedConfigRoundTrip round-trips a config holding both single- and
// multi-forward tunnels and confirms every tunnel's Forwards survive intact.
func TestSaveMixedConfigRoundTrip(t *testing.T) {
	src := writeConfig(t, `
[[tunnels]]
name   = "dev"
host   = "devhost"
local  = "9000"
remote = "localhost:9000"

[[tunnels]]
name = "prod"
host = "bastion"

  [[tunnels.forward]]
  name   = "db"
  local  = "5432"
  remote = "db.internal:5432"

  [[tunnels.forward]]
  local  = "6379"
  remote = "redis.internal:6379"

[[tunnels]]
name  = "socks"
host  = "vps"
local = "1080"
mode  = "socks"
`)
	loaded, err := loadFrom(src)
	if err != nil {
		t.Fatalf("initial loadFrom: %v", err)
	}

	out := filepath.Join(t.TempDir(), ".boring.toml")
	if err := Save(loaded, out); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	// The "[SOCKS]" placeholder is a render-time display value only
	// (tunnel.Forward.DisplayLocal / DisplayRemote); a socks forward stores its
	// unused address side empty. The placeholder must never reach disk.
	if strings.Contains(string(data), "[SOCKS]") {
		t.Fatalf("saved config leaked the [SOCKS] placeholder:\n%s", data)
	}
	reloaded, err := loadFrom(out)
	if err != nil {
		t.Fatalf("reload after Save: %v", err)
	}
	assertForwardsEqual(t, loaded.Tunnels, reloaded.Tunnels)
}

// assertForwardsEqual fails the test unless want and got describe the same
// tunnels with identical Forwards (per-forward name/local/remote/mode).
func assertForwardsEqual(t *testing.T, want, got []tunnel.Desc) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("tunnel count: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.Name != g.Name {
			t.Fatalf("tunnel[%d] name: want %q, got %q", i, w.Name, g.Name)
		}
		if len(w.Forwards) != len(g.Forwards) {
			t.Fatalf("tunnel %q forward count: want %d, got %d",
				w.Name, len(w.Forwards), len(g.Forwards))
		}
		for j := range w.Forwards {
			if w.Forwards[j] != g.Forwards[j] {
				t.Errorf("tunnel %q Forwards[%d]: want %+v, got %+v",
					w.Name, j, w.Forwards[j], g.Forwards[j])
			}
		}
	}
}

// hasShorthandBeforeForwardBlock reports whether the saved config assigns a
// local/remote key before its first [[tunnels.forward]] header. The encoder
// writes a tunnel's own keys before its forward blocks, so such a key can only
// be a tunnel-level shorthand assignment.
func hasShorthandBeforeForwardBlock(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[tunnels.forward]]" {
			return false
		}
		if strings.HasPrefix(trimmed, "local ") ||
			strings.HasPrefix(trimmed, "local=") ||
			strings.HasPrefix(trimmed, "remote ") ||
			strings.HasPrefix(trimmed, "remote=") {
			return true
		}
	}
	return false
}

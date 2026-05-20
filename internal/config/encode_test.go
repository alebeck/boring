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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

const fileName = ".boring.toml"

// SocksLabel is the display-only placeholder substituted for a socks tunnel's
// remote address (and a reverse-socks tunnel's local address) when a config is
// loaded. Those addresses are unused for socks modes; the label keeps the list
// view readable. It must never be written back to the config file.
const SocksLabel = "[SOCKS]"

var defaultKeepAliveInterval = 2 * 60 // seconds

var Path string

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	// Tunnels is a list of tunnel descriptions
	Tunnels []tunnel.Desc `toml:"tunnels"`
	// KeepAlive allows to specify a global keep alive interval,
	// (in seconds) overriding the default one. `0` indicates
	// no keep alive.
	KeepAlive  *int                    `toml:"keep_alive,omitempty"`
	TunnelsMap map[string]*tunnel.Desc `toml:"-"`
}

func init() {
	if Path = os.Getenv("BORING_CONFIG"); Path == "" {
		Path = filepath.Join(getConfigHome(), fileName)
	}
	Path = filepath.ToSlash(Path)
	Path = paths.ReplaceTilde(Path)
}

func getConfigHome() string {
	if runtime.GOOS == "linux" {
		// Follow XDG specification on Linux
		h := os.Getenv("XDG_CONFIG_HOME")
		if h == "" {
			h = "~/.config"
		}
		return filepath.Join(h, "boring")
	}
	return "~"
}

// Load parses the boring configuration file at the default path.
func Load() (*Config, error) {
	return loadFrom(Path)
}

// loadFrom parses the boring configuration file at the given path.
func loadFrom(path string) (*Config, error) {
	cfg := Config{KeepAlive: &defaultKeepAliveInterval}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("could not decode config file: %w", err)
	}

	// Set global keep alive interval for all tunnels
	// that don't specify one on their own.
	for i := range cfg.Tunnels {
		t := &cfg.Tunnels[i]
		if t.KeepAlive == nil {
			t.KeepAlive = cfg.KeepAlive
		}
	}

	// Normalize every tunnel so Forwards is always populated (length >= 1).
	// A tunnel with no [[tunnels.forward]] blocks gets a single implicit
	// forward built from the legacy local/remote/mode shorthand. This runs
	// before the socks-label rewrite below so the implicit forward captures
	// the real addresses rather than the SocksLabel placeholder.
	for i := range cfg.Tunnels {
		normalizeForwards(&cfg.Tunnels[i])
	}

	// Create a map of tunnel names to tunnel pointers for easy lookup later
	m, err := buildTunnelsMap(cfg.Tunnels)
	if err != nil {
		return nil, err
	}

	// Replace the remote address of Socks tunnels and local address of reverse
	// socks tunnels by a fixed indicator, it is not used for anything anyway
	for _, t := range m {
		switch t.Mode {
		case tunnel.Socks:
			t.RemoteAddress = SocksLabel
		case tunnel.RemoteSocks:
			t.LocalAddress = SocksLabel
		}
	}

	cfg.TunnelsMap = m
	return &cfg, nil
}

// normalizeForwards ensures a tunnel's Forwards slice has at least one entry.
// If no [[tunnels.forward]] blocks were given, it builds a single implicit
// forward from the legacy local/remote/mode shorthand. A tunnel that already
// has forwards is left unchanged.
func normalizeForwards(t *tunnel.Desc) {
	if len(t.Forwards) > 0 {
		return
	}
	t.Forwards = []tunnel.Forward{{
		LocalAddress:  t.LocalAddress,
		RemoteAddress: t.RemoteAddress,
		Mode:          t.Mode,
	}}
}

func buildTunnelsMap(tunnels []tunnel.Desc) (map[string]*tunnel.Desc, error) {
	if err := Validate(tunnels); err != nil {
		return nil, err
	}
	m := make(map[string]*tunnel.Desc)
	for i := range tunnels {
		t := &tunnels[i]
		m[t.Name] = t
	}
	return m, nil
}

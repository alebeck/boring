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

	// Validate the raw, pre-normalization state of each tunnel: a tunnel must
	// define at least one forward and must not mix the legacy local/remote
	// shorthand with [[tunnels.forward]] blocks. This runs before
	// normalizeForwards folds the shorthand away, since both checks need to
	// know which form was actually used in the file.
	for i := range cfg.Tunnels {
		if err := validateRawForwards(&cfg.Tunnels[i]); err != nil {
			return nil, err
		}
	}

	// Normalize every tunnel so Forwards is always populated (length >= 1).
	// A tunnel with no [[tunnels.forward]] blocks gets a single implicit
	// forward built from the legacy local/remote/mode shorthand. The [SOCKS]
	// display placeholder for socks modes is derived at render time
	// (tunnel.Forward.DisplayLocal / DisplayRemote), so the forward keeps the
	// real (possibly empty) addresses here.
	for i := range cfg.Tunnels {
		normalizeForwards(&cfg.Tunnels[i])
	}

	// Create a map of tunnel names to tunnel pointers for easy lookup later
	m, err := buildTunnelsMap(cfg.Tunnels)
	if err != nil {
		return nil, err
	}

	cfg.TunnelsMap = m
	return &cfg, nil
}

// validateRawForwards checks a tunnel's forward declaration before the legacy
// shorthand is folded into Forwards. It enforces two rules that depend on the
// raw form used in the config file:
//
//   - A tunnel must not set both the legacy local/remote shorthand and one or
//     more [[tunnels.forward]] blocks.
//   - A tunnel must define at least one forward, via either form.
//
// Only the addresses are consulted to decide whether the shorthand was used:
// mode defaults to its zero value and so cannot reliably signal "set".
func validateRawForwards(t *tunnel.Desc) error {
	hasShorthand := t.LocalAddress != "" || t.RemoteAddress != ""
	hasBlocks := len(t.Forwards) > 0
	if hasShorthand && hasBlocks {
		return fmt.Errorf("tunnel %q: set either local/remote or "+
			"[[tunnels.forward]], not both", t.Name)
	}
	if !hasShorthand && !hasBlocks {
		return fmt.Errorf("tunnel %q: no forward defined", t.Name)
	}
	return nil
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

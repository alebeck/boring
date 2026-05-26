package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	fileName   = ".boring.toml"
	socksLabel = "[SOCKS]"
)

var defaultKeepAliveInterval = 2 * 60 // seconds

var Path string

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	// Tunnels is a list of tunnel descriptions
	Tunnels []tunnel.Desc `toml:"tunnels"`
	// KeepAlive allows to specify a global keep alive interval,
	// (in seconds) overriding the default one. `0` indicates
	// no keep alive.
	KeepAlive  *int                    `toml:"keep_alive"`
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

// Load parses the boring configuration file
func Load() (*Config, error) {
	cfg := Config{KeepAlive: &defaultKeepAliveInterval}

	if _, err := toml.DecodeFile(Path, &cfg); err != nil {
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

	// Expand environment variable references in these fields:
	// Host
	// Name
	// LocalAddress
	// RemoteAddress
	// User
	// IdentityFile
	// Group
	for i := range cfg.Tunnels {
		cfg.Tunnels[i].Host = os.Expand(cfg.Tunnels[i].Host, expandWithDefault)
		cfg.Tunnels[i].Name = os.Expand(cfg.Tunnels[i].Name, expandWithDefault)
		cfg.Tunnels[i].LocalAddress = tunnel.StringOrInt(os.Expand(cfg.Tunnels[i].LocalAddress.String(), expandWithDefault))
		cfg.Tunnels[i].RemoteAddress = tunnel.StringOrInt(os.Expand(cfg.Tunnels[i].RemoteAddress.String(), expandWithDefault))
		cfg.Tunnels[i].User = os.Expand(cfg.Tunnels[i].User, expandWithDefault)
		cfg.Tunnels[i].IdentityFile = os.Expand(cfg.Tunnels[i].IdentityFile, expandWithDefault)
		cfg.Tunnels[i].Group = os.Expand(cfg.Tunnels[i].Group, expandWithDefault)
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
			t.RemoteAddress = socksLabel
		case tunnel.RemoteSocks:
			t.LocalAddress = socksLabel
		}
	}

	cfg.TunnelsMap = m
	return &cfg, nil
}

func buildTunnelsMap(tunnels []tunnel.Desc) (map[string]*tunnel.Desc, error) {
	m := make(map[string]*tunnel.Desc)
	for i := range tunnels {
		t := &tunnels[i]
		if _, exists := m[t.Name]; exists {
			return nil, fmt.Errorf("found duplicated tunnel name '%v'", t.Name)
		}
		if t.Name == "" || strings.Contains(t.Name, " ") ||
			specialPrefix(t.Name) || containsGlob(t.Name) {
			return nil, fmt.Errorf("tunnel names cannot be empty, contain spaces,"+
				" start with special characters, or contain glob characters '*?['."+
				" Found '%v'.", t.Name)
		}
		if t.Group != "" && (strings.Contains(t.Group, " ") ||
			specialPrefix(t.Group) || containsGlob(t.Group) || t.Group == "default") {
			return nil, fmt.Errorf("groups cannot be named 'default', contain spaces,"+
				" start with special characters, or contain glob characters '*?['."+
				" Found '%v'.", t.Group)
		}
		m[t.Name] = t
	}
	return m, nil
}

func specialPrefix(s string) bool {
	if s == "" {
		return false
	}
	firstChar := s[0]
	return !(firstChar >= 'A' && firstChar <= 'Z' ||
		firstChar >= 'a' && firstChar <= 'z' ||
		firstChar >= '0' && firstChar <= '9')
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// expandWithDefault resolves an environment variable reference, supporting
// the ${VAR:-default} syntax. If the variable is unset or empty and a default
// is provided after ":-", the default value is returned.
func expandWithDefault(key string) string {
	if idx := strings.Index(key, ":-"); idx != -1 {
		varName := key[:idx]
		defaultVal := key[idx+2:]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return defaultVal
	}
	return os.Getenv(key)
}

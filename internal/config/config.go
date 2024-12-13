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

var Path string

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
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
	var config Config
	if _, err := toml.DecodeFile(Path, &config); err != nil {
		return nil, fmt.Errorf("could not decode config file: %w", err)
	}

	// Create a map of tunnel names to tunnel pointers for easy lookup
	m := make(map[string]*tunnel.Tunnel)
	for i := range config.Tunnels {
		t := &config.Tunnels[i]
		if _, exists := m[t.Name]; exists {
			return nil, fmt.Errorf("found duplicated tunnel name '%v'", t.Name)
		}
		if t.Name == "" || strings.Contains(t.Name, " ") ||
			specialPrefix(t.Name) || containsGlob(t.Name) {
			return nil, fmt.Errorf("tunnel names cannot be empty, contain spaces,"+
				" start with special characters, or contain glob characters \"*?[\"."+
				" Found '%v'", t.Name)
		}
		m[t.Name] = t
	}

	// Replace the remote address of Socks tunnels and local address of reverse
	// socks tunnels by a fixed indicator, it is not used for anything anyway
	for _, t := range m {
		if t.Mode == tunnel.Socks {
			t.RemoteAddress = socksLabel
		} else if t.Mode == tunnel.RemoteSocks {
			t.LocalAddress = socksLabel
		}
	}

	config.TunnelsMap = m
	return &config, nil
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

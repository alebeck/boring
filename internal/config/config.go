package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	defaultFileName = "~/.boring.toml"
	socksLabel      = "[SOCKS]"
)

var FileName string

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
}

func init() {
	if FileName = os.Getenv("BORING_CONFIG"); FileName == "" {
		FileName = defaultFileName
	}
	FileName = filepath.ToSlash(FileName)
	FileName = paths.ReplaceTilde(FileName)

	// If FileName exists and is a directory, append file name
	if fi, err := os.Stat(FileName); err == nil && fi.IsDir() {
		FileName = filepath.Join(FileName, filepath.Base(defaultFileName))
	}
}

// LoadConfig parses the boring configuration file
func LoadConfig() (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(FileName, &config); err != nil {
		return nil, fmt.Errorf("could not decode config file: %v", err)
	}

	// Create a map of tunnel names to tunnel pointers for easy lookup
	m := make(map[string]*tunnel.Tunnel)
	for i := range config.Tunnels {
		t := &config.Tunnels[i]
		if _, exists := m[t.Name]; exists {
			return nil, fmt.Errorf("found duplicated tunnel name '%v'", t.Name)
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

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/tunnel"
)

const FileName = ".boring.toml"

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
}

// LoadConfig parses the boring configuration file
func LoadConfig() (*Config, error) {
	var config Config
	confPath := filepath.Join(os.Getenv("HOME"), FileName)
	if _, err := toml.DecodeFile(confPath, &config); err != nil {
		return nil, fmt.Errorf("could not decode config file: %v", err)
	}

	// Create a map of tunnel names to tunnel instances for easy lookup
	m := make(map[string]*tunnel.Tunnel)
	for _, t := range config.Tunnels {
		if _, exists := m[t.Name]; exists {
			return nil, fmt.Errorf("found duplicated tunnel name '%v'", t.Name)
		}
		m[t.Name] = &t
	}

	config.TunnelsMap = m
	return &config, nil
}

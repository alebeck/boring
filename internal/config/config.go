package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

const defaultFileName = "~/.boring.toml"

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
}

// LoadConfig parses the boring configuration file
func LoadConfig() (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(FileName, &config); err != nil {
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

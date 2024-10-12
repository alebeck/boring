package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

var FileName string

// Config represents the application configuration as parsed from $XDG_CONFIG_HOME, ~/.boring.toml or $BORING_CONFIG
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
}

func init() {
	XDGconfigPath, err := xdg.ConfigFile("boring/config.toml")
	if err != nil {
		fmt.Errorf(
			"Failed to create config in $XDG_CONFIG_HOME\n %v \n Checking for $BORING_CONFIG and then falling back to ~/.boring.toml",
		)
	}

	if FileName = os.Getenv("BORING_CONFIG"); FileName == "" {
		FileName = XDGconfigPath
		defaultFileName := paths.ReplaceTilde("~/.boring.toml")
		if _, err := os.Stat(defaultFileName); err == nil {
			FileName = defaultFileName
		}
	}
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

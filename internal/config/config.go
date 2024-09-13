package config

import (
	"fmt"
	"os/user"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/tunnel"
)

const CONFIG_FILE_NAME = ".boring.toml"

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
}

// LoadConfig reads the configuration file from the user's home directory
func LoadConfig() (*Config, error) {
	var config Config
	user, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("could not get current user: %v", err)
	}
	confPath := filepath.Join(user.HomeDir, CONFIG_FILE_NAME)
	if _, err := toml.DecodeFile(confPath, &config); err != nil {
		return nil, fmt.Errorf("could not decode config file: %v", err)
	}

	// Create a map of tunnel names to tunnel instances for easy lookup
	config.TunnelsMap = make(map[string]*tunnel.Tunnel)
	for _, t := range config.Tunnels {
		config.TunnelsMap[t.Name] = &t
	}

	return &config, nil
}

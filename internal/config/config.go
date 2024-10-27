package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/paths"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	fileName   = "boring.toml"
	socksLabel = "[SOCKS]"
)

var FilePath string

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels    []tunnel.Tunnel           `toml:"tunnels"`
	TunnelsMap map[string]*tunnel.Tunnel `toml:"-"`
}

func init() {
	FilePath = os.Getenv("BORING_CONFIG")
	if FilePath == "" {
		configHome := getConfigHome()
		FilePath = path.Join(configHome, fileName)
	}
	FilePath = filepath.ToSlash(FilePath)
	FilePath = paths.ReplaceTilde(FilePath)
}

func Ensure() error {
	if _, statErr := os.Stat(FilePath); statErr != nil {
		d := filepath.Dir(FilePath)
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(FilePath, os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			return err
		}
		f.Close()
		log.Infof("Created boring config file: %s", FilePath)
	}
	return nil
}

func Load() (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(FilePath, &config); err != nil {
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

func getConfigHome() string {
	if runtime.GOOS != "windows" {
		// Follow XDG specification on Unix
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(os.Getenv("HOME"), ".config")
		}
		return filepath.Join(configHome, "boring")
	}
	configHome := os.Getenv("LOCALAPPDATA")
	if configHome == "" {
		configHome = os.Getenv("APPDATA")
	}
	if configHome == "" {
		configHome = os.Getenv("USERPROFILE")
	}
	return filepath.Join(configHome, "boring")
}

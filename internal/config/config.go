package config

import "github.com/alebeck/boring/internal/tunnel"

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels []tunnel.Tunnel `toml:"tunnels"`
}

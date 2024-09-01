package main

// Config represents the application configuration as parsed from ./boring.toml
type Config struct {
	Tunnels []Tunnel `toml:"tunnels"`
}

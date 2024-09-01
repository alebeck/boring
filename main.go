package main

import (
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/log"
)

func main() {
	var conf config.Config
	if _, err := toml.DecodeFile("example_config.toml", &conf); err != nil {
		log.Fatalf("Error parsing TOML file: %v", err)
	}

	for _, t := range conf.Tunnels {
		if err := t.Start(); err != nil {
			log.Fatalf("Error starting tunnel: %v", err)
		}

		go func() {
			<-t.Disconnected
			log.Infof("Detected that the SSH tunnel has disconnected!")
			// Handle reconnection or other logic here
		}()

		time.Sleep(15 * time.Second) // Simulate doing some work

		if err := t.Stop(); err != nil {
			log.Fatalf("Error stopping tunnel: %v", err)
		}
	}
}

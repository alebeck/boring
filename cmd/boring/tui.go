package main

import (
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tui"
)

// runTUI loads the config, ensures the daemon is running, and launches the
// interactive terminal UI.
func runTUI() {
	conf, err := prepare()
	if err != nil {
		log.Fatalf("Startup: %s", err.Error())
	}
	if err := tui.Run(conf); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

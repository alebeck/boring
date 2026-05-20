package main

import (
	"fmt"
	"io"
	"os"

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
	// The TUI takes over the terminal with an alternate screen. Route the
	// logger away from stdout so stray log lines from background work (e.g. a
	// test connection) cannot corrupt the display.
	log.Init(io.Discard, false, false)
	if err := tui.Run(conf); err != nil {
		fmt.Fprintf(os.Stderr, "boring: TUI error: %v\n", err)
		os.Exit(1)
	}
}

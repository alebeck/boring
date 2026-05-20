package main

import (
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tui"
)

// runTUI launches the interactive terminal UI.
func runTUI() {
	if err := tui.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

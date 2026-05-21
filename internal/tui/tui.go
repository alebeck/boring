// Package tui implements the boring interactive terminal UI.
package tui

import (
	"fmt"

	"github.com/alebeck/boring/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI dashboard. A panic inside the Bubble Tea runtime is
// recovered and returned as an error: Bubble Tea restores the terminal from
// raw mode before re-panicking, so the deferred recover here keeps a panic
// from taking down the whole process with a corrupted terminal.
func Run(conf *config.Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tui panic: %v", r)
		}
	}()
	prompter := &tuiPrompter{}
	p := tea.NewProgram(newDashboard(conf, prompter), tea.WithAltScreen())
	// Safe to set send now: nothing can call Prompt until the program runs,
	// and tea.NewProgram copies the dashboard value but shares the prompter
	// pointer, so the running model sees this assignment.
	prompter.send = p.Send
	_, err = p.Run()
	return err
}

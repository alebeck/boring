// Package tui implements the boring interactive terminal UI.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// model is the root Bubble Tea model.
type model struct{}

func (m model) Init() tea.Cmd { return nil }

// Update handles incoming messages. It quits on q or ctrl+c.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	return "boring\n\nPress q to quit.\n"
}

// Run starts the TUI program. A panic inside the Bubble Tea runtime is
// recovered and returned as an error: Bubble Tea restores the terminal from
// raw mode before re-panicking, so the deferred recover here keeps a panic
// from taking down the whole process with a corrupted terminal.
func Run() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tui panic: %v", r)
		}
	}()
	_, err = tea.NewProgram(model{}, tea.WithAltScreen()).Run()
	return err
}

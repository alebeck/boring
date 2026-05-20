package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestModelQuitsOnCtrlC(t *testing.T) {
	_, cmd := model{}.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Fatal("ctrl+c should return tea.Quit")
	}
}

func TestModelQuitsOnQ(t *testing.T) {
	_, cmd := model{}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !isQuit(cmd) {
		t.Fatal("q should return tea.Quit")
	}
}

func TestModelIgnoresOtherKeys(t *testing.T) {
	_, cmd := model{}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if isQuit(cmd) {
		t.Fatal("an unrelated key should not quit")
	}
}

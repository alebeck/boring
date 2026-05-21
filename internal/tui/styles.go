package tui

import (
	"github.com/alebeck/boring/internal/tunnel"
	"github.com/charmbracelet/lipgloss"
)

// Dashboard styles. This file is intentionally limited to styling concerns.
var (
	titleStyle     = lipgloss.NewStyle().Bold(true)
	headerStyle    = lipgloss.NewStyle().Bold(true).Faint(true)
	cursorStyle    = lipgloss.NewStyle().Reverse(true)
	dimStyle       = lipgloss.NewStyle().Faint(true)
	statusBarStyle = lipgloss.NewStyle().Faint(true)
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	modalStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			Width(56) // wrap server-controlled instruction/question text
	modalTitleStyle = lipgloss.NewStyle().Bold(true)
	formLabelStyle  = lipgloss.NewStyle().Bold(true).Faint(true)
)

// branchStyle dims the ├/└ tree-branch glyphs on forward sub-rows so the tree
// structure stays visible but recedes behind the content.
var branchStyle = dimStyle

// statusStyles maps a tunnel status to the style used for its indicator and
// label: open green, reconn yellow, needs-auth magenta, closed dim.
var statusStyles = map[tunnel.Status]lipgloss.Style{
	tunnel.Open:      lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	tunnel.Reconn:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	tunnel.NeedsAuth: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
	tunnel.Closed:    lipgloss.NewStyle().Faint(true),
}

// styleForStatus returns the style for a status, falling back to dimStyle.
func styleForStatus(s tunnel.Status) lipgloss.Style {
	if st, ok := statusStyles[s]; ok {
		return st
	}
	return dimStyle
}

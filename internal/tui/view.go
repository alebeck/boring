package tui

import (
	"strings"

	"github.com/alebeck/boring/internal/tunnel"
	"github.com/charmbracelet/lipgloss"
)

// columns lists the table headers in display order.
var columns = []string{"Status", "Name", "Local", "Mode", "Remote", "Via"}

// statusText returns the human-readable label for a tunnel status.
func statusText(s tunnel.Status) string {
	switch s {
	case tunnel.Open:
		return "open"
	case tunnel.Reconn:
		return "reconn"
	case tunnel.NeedsAuth:
		return "needs auth"
	default:
		return "closed"
	}
}

// View renders the whole dashboard.
func (d dashboard) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("boring"))
	b.WriteString("\n\n")
	if d.authModal != nil {
		b.WriteString(d.authModalView())
	} else if d.showHelp {
		b.WriteString(helpView())
	} else {
		b.WriteString(d.tableView())
	}
	b.WriteString("\n")
	b.WriteString(d.statusBar())
	b.WriteString("\n")
	return b.String()
}

// authModalView renders the active auth modal, centered when the window size
// is known.
func (d dashboard) authModalView() string {
	modal := d.authModal.View()
	if d.width > 0 && d.height > 0 {
		return lipgloss.Place(d.width, lipgloss.Height(modal),
			lipgloss.Center, lipgloss.Center, modal)
	}
	return modal
}

// cells returns the raw (unstyled) cell text for one tunnel row.
func cells(t *tunnel.Desc) []string {
	return []string{
		statusText(t.Status),
		t.Name,
		t.LocalAddress.String(),
		t.Mode.String(),
		t.RemoteAddress.String(),
		t.Host,
	}
}

// columnWidths computes each column's width as the max of the header and all
// cell widths for that column.
func columnWidths(rows []*tunnel.Desc) []int {
	widths := make([]int, len(columns))
	for i, h := range columns {
		widths[i] = lipgloss.Width(h)
	}
	for _, t := range rows {
		for i, c := range cells(t) {
			if w := lipgloss.Width(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// tableView renders the tunnel table, or a placeholder if empty.
func (d dashboard) tableView() string {
	if len(d.rows) == 0 {
		return dimStyle.Render("No tunnels configured.")
	}
	widths := columnWidths(d.rows)
	var b strings.Builder
	b.WriteString(headerRow(widths))
	for i, t := range d.rows {
		b.WriteString("\n")
		b.WriteString(renderRow(t, widths, i == d.cursor))
	}
	return b.String()
}

// pad right-pads s with spaces to the given width.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// headerRow renders the aligned column header line.
func headerRow(widths []int) string {
	parts := make([]string, len(columns))
	for i, h := range columns {
		parts[i] = pad(h, widths[i])
	}
	return headerStyle.Render(strings.Join(parts, "  "))
}

// renderRow renders one tunnel row, highlighting it when it is the cursor row.
func renderRow(t *tunnel.Desc, widths []int, selected bool) string {
	raw := cells(t)
	parts := make([]string, len(raw))
	for i, c := range raw {
		parts[i] = pad(c, widths[i])
	}
	// Color the status cell unless the whole row is highlighted.
	if !selected {
		parts[0] = styleForStatus(t.Status).Render(parts[0])
	}
	line := strings.Join(parts, "  ")
	if selected {
		return cursorStyle.Render(line)
	}
	return line
}

// statusBar renders the bottom bar: a status message, or a key hint.
func (d dashboard) statusBar() string {
	if d.status == "" {
		return statusBarStyle.Render("j/k move · enter open/close · ? help · q quit")
	}
	if strings.HasPrefix(d.status, daemonUnavailablePrefix) {
		return errStyle.Render(d.status)
	}
	return statusBarStyle.Render(d.status)
}

// helpView renders the key reference shown when help is toggled.
func helpView() string {
	lines := []string{
		"Keys:",
		"  up/k     move cursor up",
		"  down/j   move cursor down",
		"  enter    open/close selected tunnel",
		"  space    open/close selected tunnel",
		"  ?        toggle this help",
		"  q        quit",
		"  ctrl+c   quit",
	}
	return dimStyle.Render(strings.Join(lines, "\n"))
}

package tui

import (
	"fmt"
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

// View renders the whole dashboard: a body region (title + table or active
// modal) and a fixed footer pinned to the bottom of the terminal.
func (d dashboard) View() string {
	body := d.bodyView()
	footer := d.footerView()
	if d.height <= 0 {
		// Window size not known yet; render without bottom-pinning.
		return body + "\n\n" + footer
	}
	// Pad between the body and the footer so the footer rests on the last
	// rows of the terminal, like a fixed tool bar.
	gap := d.height - lipgloss.Height(body) - lipgloss.Height(footer)
	if gap < 1 {
		gap = 1
	}
	return body + "\n" + strings.Repeat("\n", gap) + footer
}

// bodyView renders the title and the main content: the tunnel table, or the
// active modal / form / help screen.
func (d dashboard) bodyView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("boring"))
	b.WriteString("\n\n")
	switch {
	case d.authModal != nil:
		b.WriteString(d.authModalView())
	case d.form != nil:
		b.WriteString(d.form.View())
	case d.confirmDelete != "":
		b.WriteString(d.deleteConfirmView())
	case d.testResult != nil:
		b.WriteString(d.testResultModalView())
	case d.showHelp:
		b.WriteString(helpView())
	default:
		b.WriteString(d.tableView())
	}
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

// deleteConfirmView renders the delete confirmation box, centered when the
// window size is known.
func (d dashboard) deleteConfirmView() string {
	box := confirmView(fmt.Sprintf("Delete tunnel %q?", d.confirmDelete))
	if d.width > 0 && d.height > 0 {
		return lipgloss.Place(d.width, lipgloss.Height(box),
			lipgloss.Center, lipgloss.Center, box)
	}
	return box
}

// testResultModalView renders the connection-test result box, centered when the
// window size is known.
func (d dashboard) testResultModalView() string {
	box := testResultView(*d.testResult)
	if d.width > 0 && d.height > 0 {
		return lipgloss.Place(d.width, lipgloss.Height(box),
			lipgloss.Center, lipgloss.Center, box)
	}
	return box
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

// footerView renders the fixed bottom area: a separator rule, a message line
// (the transient status/error — blank when there is none, so the line below
// never shifts), and a context key-hint line that is always shown. The
// key hints therefore never disappear behind a status message.
func (d dashboard) footerView() string {
	width := d.width
	if width <= 0 {
		width = 80
	}
	sep := dimStyle.Render(strings.Repeat("─", width))
	return sep + "\n" + d.messageLine() + "\n" + statusBarStyle.Render(d.keyHint())
}

// messageLine renders the transient status/error message as a single line —
// blank when there is no message — so the key-hint line never moves.
func (d dashboard) messageLine() string {
	if d.status == "" {
		return ""
	}
	msg := strings.ReplaceAll(d.status, "\n", " ")
	if strings.HasPrefix(msg, daemonUnavailablePrefix) {
		return errStyle.Render(msg)
	}
	return statusBarStyle.Render(msg)
}

// keyHint returns the key reference for the current context. It is always
// shown in the footer, so the commands never disappear behind a message.
func (d dashboard) keyHint() string {
	switch {
	case d.authModal != nil:
		return "enter submit · esc cancel · ctrl+c quit"
	case d.form != nil:
		return formHint
	case d.confirmDelete != "":
		return "y confirm · n / esc cancel · ctrl+c quit"
	case d.testResult != nil:
		return "any key dismisses · ctrl+c quit"
	case d.showHelp:
		return "? close help · q quit"
	default:
		return "j/k move · enter open/close · t test · a add · e edit · d delete · ? help · q quit"
	}
}

// helpView renders the key reference shown when help is toggled.
func helpView() string {
	lines := []string{
		"Keys:",
		"  up/k     move cursor up",
		"  down/j   move cursor down",
		"  enter    open/close selected tunnel",
		"  space    open/close selected tunnel",
		"  t        test the selected tunnel's connection",
		"  a        add a new tunnel",
		"  e        edit the selected tunnel",
		"  d        delete the selected tunnel",
		"  ?        toggle this help",
		"  q        quit",
		"  ctrl+c   quit",
	}
	return dimStyle.Render(strings.Join(lines, "\n"))
}

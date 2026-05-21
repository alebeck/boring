package tui

import (
	"fmt"
	"strings"

	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"
	"github.com/charmbracelet/lipgloss"
)

// The grouped-tree vocabulary — the column headers and the branch glyphs — is
// owned by internal/table (the CLI `boring list` renderer) and shared here so
// the dashboard and the CLI draw identical trees: table.TunnelColumns,
// table.BranchMid, table.BranchLast, table.SubIndent.

// Status-indicator glyphs: a filled dot for the live states (open, reconn,
// needs auth) and a hollow dot for closed. The glyph alone cannot tell the
// live states apart, so it is always shown next to the status word.
const (
	indicatorLive   = "●"
	indicatorClosed = "○"
)

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

// statusIndicator returns the dot glyph for a tunnel status: filled for the
// live states, hollow for closed.
func statusIndicator(s tunnel.Status) string {
	if s == tunnel.Closed {
		return indicatorClosed
	}
	return indicatorLive
}

// statusCell returns the raw (unstyled) Status-column text: the indicator dot
// followed by the status word, e.g. "● open" or "○ closed". renderLine colours
// the whole cell per status; columnWidths sizes the column from this text.
func statusCell(s tunnel.Status) string {
	return statusIndicator(s) + " " + statusText(s)
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

// forwardLabel returns the indented Name-column text of a forward sub-row: the
// indent, the branch glyph, and the forward's label.
func forwardLabel(f tunnel.Forward, last bool) string {
	branch := table.BranchMid
	if last {
		branch = table.BranchLast
	}
	return table.SubIndent + branch + " " + f.Label()
}

// inlineCells returns the raw cell text for a single-forward tunnel rendered on
// one line: status, name, the forward's local/mode/remote, and the via host.
func inlineCells(t *tunnel.Desc) []string {
	f := t.Forwards[0]
	return []string{
		statusCell(t.Status),
		t.Name,
		f.DisplayLocal(),
		f.Mode.String(),
		f.DisplayRemote(),
		t.Host,
	}
}

// headerCells returns the raw cell text for a multi-forward tunnel's
// connection-level row: status, name, and via host. The forward columns are
// blank — the forwards follow as indented sub-rows.
func headerCells(t *tunnel.Desc) []string {
	return []string{statusCell(t.Status), t.Name, "", "", "", t.Host}
}

// forwardCells returns the raw cell text for one indented forward sub-row. The
// Status column is blank; the branch glyph and label occupy the Name column.
func forwardCells(f tunnel.Forward, last bool) []string {
	return []string{
		"",
		forwardLabel(f, last),
		f.DisplayLocal(),
		f.Mode.String(),
		f.DisplayRemote(),
		"",
	}
}

// observeWidths widens each column to fit the given raw cell row.
func observeWidths(widths []int, cells []string) {
	for i, c := range cells {
		if w := lipgloss.Width(c); w > widths[i] {
			widths[i] = w
		}
	}
}

// columnWidths computes each column's width as the max of the header and every
// rendered cell — inline rows, multi-forward header rows, and forward sub-rows
// — so the columns line up across the whole grouped tree.
func columnWidths(rows []*tunnel.Desc) []int {
	widths := make([]int, len(table.TunnelColumns))
	for i, h := range table.TunnelColumns {
		widths[i] = lipgloss.Width(h)
	}
	for _, t := range rows {
		if len(t.Forwards) == 1 {
			observeWidths(widths, inlineCells(t))
			continue
		}
		// A tunnel with zero or many forwards renders a connection-level
		// header row; many forwards add indented sub-rows.
		observeWidths(widths, headerCells(t))
		for i, f := range t.Forwards {
			observeWidths(widths, forwardCells(f, i == len(t.Forwards)-1))
		}
	}
	return widths
}

// tableView renders the tunnel table, or a placeholder if empty. Each tunnel
// expands into one inline line (single forward) or a header line plus one
// indented sub-row per forward (multi-forward). The cursor selects tunnels:
// the whole block of the selected tunnel is highlighted.
func (d dashboard) tableView() string {
	if len(d.rows) == 0 {
		return dimStyle.Render("No tunnels configured.")
	}
	widths := columnWidths(d.rows)
	var b strings.Builder
	b.WriteString(headerRow(widths))
	for i, t := range d.rows {
		b.WriteString("\n")
		b.WriteString(renderTunnel(t, widths, i == d.cursor))
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
	parts := make([]string, len(table.TunnelColumns))
	for i, h := range table.TunnelColumns {
		parts[i] = pad(h, widths[i])
	}
	return headerStyle.Render(strings.Join(parts, "  "))
}

// joinCells right-pads each raw cell to its column width and joins them.
func joinCells(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = pad(c, widths[i])
	}
	return strings.Join(parts, "  ")
}

// renderTunnel renders one tunnel, highlighting the whole block when it is the
// selected (cursor) tunnel. A single-forward tunnel is one inline line; a
// multi-forward tunnel is a header line plus one indented sub-row per forward.
func renderTunnel(t *tunnel.Desc, widths []int, selected bool) string {
	if len(t.Forwards) == 1 {
		return renderLine(inlineCells(t), widths, t.Status, selected, false)
	}
	lines := []string{renderLine(headerCells(t), widths, t.Status, selected, false)}
	for i, f := range t.Forwards {
		cells := forwardCells(f, i == len(t.Forwards)-1)
		lines = append(lines, renderLine(cells, widths, t.Status, selected, true))
	}
	return strings.Join(lines, "\n")
}

// renderLine renders one table line. When the line belongs to the selected
// tunnel the whole line is highlighted and no per-cell colour is applied — the
// highlight takes precedence. Otherwise the status cell is coloured per status
// and, on a forward sub-row, the tree-branch glyph is dimmed.
func renderLine(cells []string, widths []int, status tunnel.Status, selected, forward bool) string {
	if selected {
		return cursorStyle.Render(joinCells(cells, widths))
	}
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = pad(c, widths[i])
	}
	parts[0] = styleForStatus(status).Render(parts[0])
	if forward {
		parts[1] = dimBranch(parts[1])
	}
	return strings.Join(parts, "  ")
}

// dimBranch dims the ├/└ tree-branch glyph at the start of a forward sub-row's
// Name cell, leaving the indent and the label in the default foreground.
func dimBranch(nameCell string) string {
	for _, branch := range []string{table.BranchMid, table.BranchLast} {
		glyph := table.SubIndent + branch
		if rest, ok := strings.CutPrefix(nameCell, glyph); ok {
			return table.SubIndent + branchStyle.Render(branch) + rest
		}
	}
	return nameCell
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
		"  ctrl+f   add a forward (in the add/edit form)",
		"  ctrl+x   remove the focused forward (in the form)",
		"  ?        toggle this help",
		"  q        quit",
		"  ctrl+c   quit",
	}
	return dimStyle.Render(strings.Join(lines, "\n"))
}

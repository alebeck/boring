// This file renders the grouped-tree layout `boring list` uses for
// multi-forward tunnels. The generic Table in table.go is a flat header+rows
// grid and cannot express a tunnel header followed by indented forward
// sub-rows, so the tunnel-specific layout lives here, in its own focused type.

package table

import (
	"strings"
	"unicode/utf8"

	"github.com/alebeck/boring/internal/log"
)

// visibleWidth returns the printed column width of s: its rune count with ANSI
// escape sequences stripped. The generic table's length() counts bytes, which
// is fine for ASCII; the grouped tree uses the multi-byte ├/└ glyphs, so this
// renderer needs rune-accurate widths to keep columns aligned.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansi.ReplaceAllString(s, ""))
}

// Tree-branch glyphs for forward sub-rows. These are the shared grouped-tree
// vocabulary: the TUI renderer (internal/tui) imports them so the CLI list and
// the dashboard draw identical trees.
const (
	BranchMid  = "├"  // ├ — a forward that is not the last
	BranchLast = "└"  // └ — the last forward of a tunnel
	SubIndent  = "  " // indent before a forward sub-row's branch glyph
)

// TunnelRow is one tunnel in a grouped listing: a status, a name, a "via"
// host, and one or more forwards. A single-forward tunnel renders inline on
// one line; a multi-forward tunnel renders a header line plus one indented
// sub-row per forward.
type TunnelRow struct {
	Status   string
	Name     string
	Via      string
	Forwards []ForwardRow
}

// ForwardRow is one forward of a tunnel: a label (the forward's name, or its
// local address when unnamed) and the local/mode/remote addresses.
type ForwardRow struct {
	Label  string
	Local  string
	Mode   string
	Remote string
}

// TunnelTable renders tunnels as a grouped tree. Column widths are computed
// across both inline single-forward rows and indented forward sub-rows so the
// Local / mode / Remote columns line up everywhere.
type TunnelTable struct {
	rows []TunnelRow
}

// NewTunnelTable returns an empty grouped tunnel table.
func NewTunnelTable() *TunnelTable {
	return &TunnelTable{}
}

// Add appends a tunnel to the table.
func (t *TunnelTable) Add(row TunnelRow) {
	t.rows = append(t.rows, row)
}

// TunnelColumns lists the grouped-tree column headers in display order. The
// mode column is unlabeled, matching the flat list table. It is the shared
// header vocabulary: the TUI renderer (internal/tui) imports it so both
// renderers stay column-for-column identical. Read-only — do not mutate.
var TunnelColumns = []string{"Status", "Name", "Local", "", "Remote", "Via"}

// widths holds the rendered width of every column of the grouped table.
type widths struct {
	status, name, local, mode, remote int
}

// String renders the grouped tunnel table, including the header row.
func (t *TunnelTable) String() string {
	w := t.columnWidths()
	var b strings.Builder
	writeHeader(&b, w)
	for _, row := range t.rows {
		writeTunnel(&b, row, w)
	}
	b.WriteString("\n")
	return b.String()
}

// columnWidths measures every column across inline rows, multi-forward
// headers, and forward sub-rows. The Name column must fit both an inline
// tunnel name and the widest indented forward label.
func (t *TunnelTable) columnWidths() widths {
	w := widths{
		status: visibleWidth(TunnelColumns[0]),
		name:   visibleWidth(TunnelColumns[1]),
		local:  visibleWidth(TunnelColumns[2]),
		mode:   visibleWidth(TunnelColumns[3]),
		remote: visibleWidth(TunnelColumns[4]),
	}
	for _, row := range t.rows {
		w.status = max(w.status, visibleWidth(row.Status))
		w.name = max(w.name, visibleWidth(row.Name))
		for _, f := range row.Forwards {
			w.name = max(w.name, visibleWidth(SubIndent)+visibleWidth(BranchMid)+1+visibleWidth(f.Label))
			w.local = max(w.local, visibleWidth(f.Local))
			w.mode = max(w.mode, visibleWidth(f.Mode))
			w.remote = max(w.remote, visibleWidth(f.Remote))
		}
	}
	return w
}

// writeHeader writes the bold column header row. The trailing Via column is
// not padded — it is the last column.
func writeHeader(b *strings.Builder, w widths) {
	for i := 0; i < len(TunnelColumns)-1; i++ {
		c := TunnelColumns[i]
		writeCell(b, log.Bold+c+log.Reset, columnWidth(w, i), visibleWidth(c))
	}
	via := TunnelColumns[len(TunnelColumns)-1]
	b.WriteString(log.Bold + via + log.Reset)
}

// writeTunnel writes one tunnel: an inline row when it has a single forward,
// or a header row plus indented forward sub-rows otherwise.
func writeTunnel(b *strings.Builder, row TunnelRow, w widths) {
	b.WriteString("\n")
	if len(row.Forwards) == 1 {
		writeInlineRow(b, row, w)
		return
	}
	writeHeaderRow(b, row, w)
	for i, f := range row.Forwards {
		b.WriteString("\n")
		writeForwardRow(b, f, w, i == len(row.Forwards)-1)
	}
}

// writeInlineRow renders a single-forward tunnel on one line: status, name,
// the forward's local/mode/remote, and the via host.
func writeInlineRow(b *strings.Builder, row TunnelRow, w widths) {
	f := row.Forwards[0]
	writeCell(b, row.Status, w.status, visibleWidth(row.Status))
	writeCell(b, row.Name, w.name, visibleWidth(row.Name))
	writeCell(b, f.Local, w.local, visibleWidth(f.Local))
	writeCell(b, f.Mode, w.mode, visibleWidth(f.Mode))
	writeCell(b, f.Remote, w.remote, visibleWidth(f.Remote))
	b.WriteString(row.Via)
}

// writeHeaderRow renders the connection-level row of a multi-forward tunnel:
// status, name, and via host. The forward columns are left blank; the
// forwards follow as indented sub-rows.
func writeHeaderRow(b *strings.Builder, row TunnelRow, w widths) {
	writeCell(b, row.Status, w.status, visibleWidth(row.Status))
	writeCell(b, row.Name, w.name, visibleWidth(row.Name))
	writeCell(b, "", w.local, 0)
	writeCell(b, "", w.mode, 0)
	writeCell(b, "", w.remote, 0)
	b.WriteString(row.Via)
}

// writeForwardRow renders one indented forward sub-row. The branch glyph and
// label occupy the Name column; the local/mode/remote columns align with the
// inline-row columns. The Status column is left blank.
func writeForwardRow(b *strings.Builder, f ForwardRow, w widths, last bool) {
	branch := BranchMid
	if last {
		branch = BranchLast
	}
	writeCell(b, "", w.status, 0)
	label := SubIndent + branch + " " + f.Label
	writeCell(b, label, w.name, visibleWidth(label))
	writeCell(b, f.Local, w.local, visibleWidth(f.Local))
	writeCell(b, f.Mode, w.mode, visibleWidth(f.Mode))
	writeCell(b, f.Remote, w.remote, visibleWidth(f.Remote))
}

// columnWidth returns the measured width of column i (0..4: status, name,
// local, mode, remote). The trailing Via column is not measured — it is the
// last column and never padded.
func columnWidth(w widths, i int) int {
	switch i {
	case 0:
		return w.status
	case 1:
		return w.name
	case 2:
		return w.local
	case 3:
		return w.mode
	default:
		return w.remote
	}
}

// writeCell writes text padded to width+pad, accounting for ANSI escape
// sequences via the supplied visible length.
func writeCell(b *strings.Builder, text string, width, visible int) {
	b.WriteString(text)
	gap := width + pad - visible
	if gap < 0 {
		gap = 0
	}
	b.WriteString(strings.Repeat(" ", gap))
}

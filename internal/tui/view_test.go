package tui

import (
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

// multiForward builds a Desc with the given named forwards (mode local). It
// mirrors a tunnel loaded from one or more [[tunnels.forward]] blocks.
func multiForward(name string, forwards ...tunnel.Forward) tunnel.Desc {
	return tunnel.Desc{Name: name, Host: "bastion", Forwards: forwards}
}

// dashboardWithDescs builds a dashboard directly from the given descriptions,
// preserving each one's Forwards slice (dashboardWithRows only makes
// single-forward tunnels).
func dashboardWithDescs(descs ...tunnel.Desc) dashboard {
	return newDashboard(&config.Config{Tunnels: descs}, &tuiPrompter{})
}

func TestTableViewSingleForwardInline(t *testing.T) {
	d := dashboardWithDescs(singleForward("dev"))
	out := d.View()
	// A single-forward tunnel renders inline: its name, local and remote all
	// on one line, with no tree-branch glyphs.
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "dev") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("view should contain a row for dev:\n%s", out)
	}
	if !strings.Contains(line, "9000") || !strings.Contains(line, "localhost:9000") {
		t.Fatalf("single-forward row should be inline with local+remote, got %q", line)
	}
	if strings.Contains(out, table.BranchMid) || strings.Contains(out, table.BranchLast) {
		t.Fatalf("single-forward tunnel must not render tree branches:\n%s", out)
	}
}

func TestTableViewMultiForwardTree(t *testing.T) {
	d := dashboardWithDescs(multiForward("prod",
		tunnel.Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
		tunnel.Forward{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379"},
	))
	out := d.View()
	lines := strings.Split(out, "\n")

	// Header line: the tunnel name and host, no forward addresses.
	var header string
	for _, l := range lines {
		if strings.Contains(l, "prod") {
			header = l
			break
		}
	}
	if header == "" {
		t.Fatalf("view should contain a header line for prod:\n%s", out)
	}
	if !strings.Contains(header, "bastion") {
		t.Fatalf("header line should show the via host, got %q", header)
	}
	if strings.Contains(header, "5432") || strings.Contains(header, "6379") {
		t.Fatalf("header line must not carry forward addresses, got %q", header)
	}

	// First forward: a ├ sub-row with its label and local → remote.
	if !lineWith(lines, table.BranchMid, "db", "5432", "db:5432") {
		t.Fatalf("expected a ├ sub-row for db with 5432/db:5432:\n%s", out)
	}
	// Last forward: a └ sub-row.
	if !lineWith(lines, table.BranchLast, "cache", "6379", "redis:6379") {
		t.Fatalf("expected a └ sub-row for cache with 6379/redis:6379:\n%s", out)
	}
}

func TestTableViewUnnamedForwardLabel(t *testing.T) {
	d := dashboardWithDescs(multiForward("prod",
		tunnel.Forward{LocalAddress: "5432", RemoteAddress: "db:5432"},
		tunnel.Forward{LocalAddress: "6379", RemoteAddress: "redis:6379"},
	))
	out := d.View()
	lines := strings.Split(out, "\n")
	// An unnamed forward falls back to its local address as the label.
	if !lineWith(lines, table.BranchMid, "5432", "db:5432") {
		t.Fatalf("unnamed forward should be labelled by its local address:\n%s", out)
	}
}

func TestRenderTunnelZeroForwards(t *testing.T) {
	// A Desc with no forwards is degenerate but must not panic: renderTunnel
	// falls through to the multi-forward path and emits the header row only.
	desc := multiForward("empty") // no forwards
	widths := columnWidths([]*tunnel.Desc{&desc})

	out := renderTunnel(&desc, widths, false)
	if strings.Contains(out, "\n") {
		t.Fatalf("zero-forward tunnel should render a single header line, got:\n%s", out)
	}
	if !strings.Contains(out, "empty") {
		t.Fatalf("zero-forward header line should carry the tunnel name, got %q", out)
	}
	if strings.Contains(out, table.BranchMid) || strings.Contains(out, table.BranchLast) {
		t.Fatalf("zero-forward tunnel must not render tree branches, got %q", out)
	}
}

// lineWith reports whether some line in lines contains every substring.
func lineWith(lines []string, subs ...string) bool {
	for _, l := range lines {
		ok := true
		for _, s := range subs {
			if !strings.Contains(l, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestTableViewCursorMovesTunnelByTunnel(t *testing.T) {
	// A multi-forward tunnel followed by a single-forward tunnel. The cursor
	// must index tunnels (2), not display lines (1 + 2 + 1 = 4).
	d := dashboardWithDescs(
		multiForward("prod",
			tunnel.Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
			tunnel.Forward{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379"},
		),
		singleForward("dev"),
	)

	if d.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", d.cursor)
	}
	// One step down lands on the second tunnel, not the second display line.
	m, _ := d.Update(keyMsg('j'))
	if got := m.(dashboard).cursor; got != 1 {
		t.Fatalf("j should move to tunnel 1, got %d", got)
	}
	// A second step down clamps at the last tunnel (index 1), proving the
	// cursor never lands on a forward sub-row.
	m, _ = m.(dashboard).Update(keyMsg('j'))
	if got := m.(dashboard).cursor; got != 1 {
		t.Fatalf("cursor should clamp at the last tunnel (1), got %d", got)
	}
	// Back up to the first tunnel.
	m, _ = m.(dashboard).Update(keyMsg('k'))
	if got := m.(dashboard).cursor; got != 0 {
		t.Fatalf("k should move back to tunnel 0, got %d", got)
	}
}

func TestRenderTunnelHighlightsSelectedBlock(t *testing.T) {
	desc := multiForward("prod",
		tunnel.Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
		tunnel.Forward{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379"},
	)
	widths := columnWidths([]*tunnel.Desc{&desc})

	plain := renderTunnel(&desc, widths, false)
	selected := renderTunnel(&desc, widths, true)

	// The selected block spans the same display lines as the unselected one
	// (1 header + 2 forwards) — highlighting must not change the line count.
	if got := strings.Count(plain, "\n"); got != 2 {
		t.Fatalf("multi-forward tunnel should span 3 lines (2 newlines), got %d", got)
	}
	if strings.Count(plain, "\n") != strings.Count(selected, "\n") {
		t.Fatal("highlighting must not change the line count of the tunnel block")
	}
	// Every line of the selected block is rendered through cursorStyle; the
	// unselected block is not. Comparing against an explicit cursorStyle
	// render is profile-agnostic: both sides take the identical style path.
	for i, line := range strings.Split(selected, "\n") {
		want := cursorStyle.Render(strings.Split(plainLines(&desc, widths), "\n")[i])
		if line != want {
			t.Fatalf("selected line %d not highlighted via cursorStyle:\n got %q\nwant %q", i, line, want)
		}
	}
}

// plainLines renders a tunnel's block with the status styling stripped — the
// raw padded cells joined per line — so a test can re-apply cursorStyle and
// compare against the highlighted render.
func plainLines(t *tunnel.Desc, widths []int) string {
	if len(t.Forwards) == 1 {
		return joinCells(inlineCells(t), widths)
	}
	lines := []string{joinCells(headerCells(t), widths)}
	for i, f := range t.Forwards {
		lines = append(lines, joinCells(forwardCells(f, i == len(t.Forwards)-1), widths))
	}
	return strings.Join(lines, "\n")
}

func TestTableViewFooterPinnedWithTallBody(t *testing.T) {
	// A multi-forward tunnel makes the body taller than a single-forward one;
	// the footer must still be pinned to the bottom of the terminal.
	d := dashboardWithDescs(multiForward("prod",
		tunnel.Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
		tunnel.Forward{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379"},
		tunnel.Forward{Name: "api", LocalAddress: "8080", RemoteAddress: "api:8080"},
	))
	d.width, d.height = 80, 24
	out := d.View()
	lines := strings.Split(out, "\n")
	if len(lines) != d.height {
		t.Fatalf("rendered view should be exactly %d lines tall, got %d", d.height, len(lines))
	}
	// The last line is the key-hint footer.
	if !strings.Contains(lines[len(lines)-1], "j/k move") {
		t.Fatalf("footer key hints should be on the last line, got %q", lines[len(lines)-1])
	}
}

func TestSelectedHandlersActOnCursorTunnel(t *testing.T) {
	// enter/d/e/t act on d.rows[d.cursor]; with a multi-forward tunnel before
	// a single-forward one, the handlers must still pick the right tunnel.
	d := dashboardWithDescs(
		multiForward("prod",
			tunnel.Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
			tunnel.Forward{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379"},
		),
		singleForward("dev"),
	)
	d.cursor = 1 // the single-forward "dev"

	// enter on a not-running tunnel starts an open and reports it.
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m.(dashboard).status; !strings.Contains(got, "Opening dev") {
		t.Fatalf("enter should open the cursor tunnel (dev), status = %q", got)
	}

	// t tests the cursor tunnel.
	m, _ = d.Update(keyMsg('t'))
	if got := m.(dashboard).status; !strings.Contains(got, "Testing dev") {
		t.Fatalf("t should test the cursor tunnel (dev), status = %q", got)
	}

	// selectedIsRunning reports on the cursor tunnel.
	d.running = map[string]*tunnel.Desc{"prod": ptrDesc(multiForward("prod"))}
	d.cursor = 0
	if !d.selectedIsRunning() {
		t.Fatal("selectedIsRunning should be true for the running multi-forward tunnel")
	}
	d.cursor = 1
	if d.selectedIsRunning() {
		t.Fatal("selectedIsRunning should be false for the non-running single-forward tunnel")
	}
}

// ptrDesc returns a pointer to a copy of d, for building running-tunnel maps.
func ptrDesc(d tunnel.Desc) *tunnel.Desc {
	return &d
}

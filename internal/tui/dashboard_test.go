package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

// isQuit reports whether running cmd yields a tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// keyMsg builds a tea.KeyMsg for a single rune key.
func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// dashboardWithRows builds a dashboard whose rows are the given descriptions.
func dashboardWithRows(names ...string) dashboard {
	descs := make([]tunnel.Desc, len(names))
	for i, n := range names {
		// Each tunnel needs a forward, otherwise config.Load rejects it on
		// the reload that follows a save (e.g. after a delete).
		descs[i] = tunnel.Desc{
			Name:          n,
			Host:          "example.com",
			LocalAddress:  "9000",
			RemoteAddress: "localhost:9000",
		}
	}
	return newDashboard(&config.Config{Tunnels: descs}, &tuiPrompter{})
}

func TestDashboardQuitsOnQ(t *testing.T) {
	_, cmd := dashboardWithRows("a").Update(keyMsg('q'))
	if !isQuit(cmd) {
		t.Fatal("q should return tea.Quit")
	}
}

func TestDashboardQuitsOnCtrlC(t *testing.T) {
	_, cmd := dashboardWithRows("a").Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Fatal("ctrl+c should return tea.Quit")
	}
}

func TestDashboardTogglesHelp(t *testing.T) {
	d := dashboardWithRows("a")
	m, _ := d.Update(keyMsg('?'))
	if !m.(dashboard).showHelp {
		t.Fatal("? should turn help on")
	}
	m, _ = m.(dashboard).Update(keyMsg('?'))
	if m.(dashboard).showHelp {
		t.Fatal("? should toggle help back off")
	}
}

func TestDashboardCursorMovesAndClamps(t *testing.T) {
	d := dashboardWithRows("a", "b", "c")

	// Moving up at the top stays clamped at 0.
	m, _ := d.Update(keyMsg('k'))
	if got := m.(dashboard).cursor; got != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", got)
	}

	// Move down twice.
	m, _ = m.(dashboard).Update(keyMsg('j'))
	m, _ = m.(dashboard).Update(keyMsg('j'))
	if got := m.(dashboard).cursor; got != 2 {
		t.Fatalf("cursor should be 2, got %d", got)
	}

	// Moving down at the bottom stays clamped at the last row.
	m, _ = m.(dashboard).Update(keyMsg('j'))
	if got := m.(dashboard).cursor; got != 2 {
		t.Fatalf("cursor should clamp at 2, got %d", got)
	}

	// Arrow keys move too.
	m, _ = m.(dashboard).Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.(dashboard).cursor; got != 1 {
		t.Fatalf("up arrow should move cursor to 1, got %d", got)
	}
}

func TestDashboardIgnoresUnrelatedKey(t *testing.T) {
	d := dashboardWithRows("a", "b", "c")
	d.cursor = 1
	m, cmd := d.Update(keyMsg('x'))
	nd := m.(dashboard)
	if isQuit(cmd) {
		t.Fatal("an unrelated key must not quit")
	}
	if nd.cursor != 1 {
		t.Fatalf("an unrelated key moved the cursor to %d, want 1", nd.cursor)
	}
	if nd.showHelp {
		t.Fatal("an unrelated key toggled help")
	}
}

func TestDashboardHandleTunnelsMergesRunning(t *testing.T) {
	d := dashboardWithRows("configured")
	running := map[string]*tunnel.Desc{
		"adhoc": {Name: "adhoc", Status: tunnel.Open},
	}
	m, cmd := d.Update(tunnelsMsg{running: running})
	if cmd == nil {
		t.Fatal("handleTunnels should schedule the next poll")
	}
	rows := m.(dashboard).rows
	if len(rows) != 2 {
		t.Fatalf("expected 2 merged rows, got %d", len(rows))
	}
	// Order: configured first, then running-unconfigured.
	if rows[0].Name != "configured" || rows[1].Name != "adhoc" {
		t.Fatalf("unexpected row order: %s, %s", rows[0].Name, rows[1].Name)
	}
}

func TestDashboardHandleTunnelsError(t *testing.T) {
	d := dashboardWithRows("a")
	m, _ := d.Update(tunnelsMsg{err: errStub{}})
	status := m.(dashboard).status
	if !strings.HasPrefix(status, daemonUnavailablePrefix) {
		t.Fatalf("error poll should set a daemon-unavailable status, got %q", status)
	}

	// A subsequent successful poll clears the error.
	m, _ = m.(dashboard).Update(tunnelsMsg{running: nil})
	if got := m.(dashboard).status; got != "" {
		t.Fatalf("successful poll should clear the error status, got %q", got)
	}
}

func TestDashboardViewContainsTunnelName(t *testing.T) {
	d := dashboardWithRows("alpha", "beta")
	out := d.View()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("view should contain tunnel names, got:\n%s", out)
	}
}

func TestDashboardViewEmpty(t *testing.T) {
	d := newDashboard(&config.Config{}, &tuiPrompter{})
	out := d.View()
	if !strings.Contains(out, "No tunnels configured.") {
		t.Fatalf("empty view should show placeholder, got:\n%s", out)
	}
}

func TestDashboardViewHelp(t *testing.T) {
	d := dashboardWithRows("a")
	m, _ := d.Update(keyMsg('?'))
	out := m.(dashboard).View()
	if !strings.Contains(out, "toggle this help") {
		t.Fatalf("help view should list keys, got:\n%s", out)
	}
}

func TestStatusText(t *testing.T) {
	cases := map[tunnel.Status]string{
		tunnel.Closed:    "closed",
		tunnel.Open:      "open",
		tunnel.Reconn:    "reconn",
		tunnel.NeedsAuth: "needs auth",
	}
	for s, want := range cases {
		if got := statusText(s); got != want {
			t.Errorf("statusText(%v) = %q, want %q", s, got, want)
		}
	}
}

func TestSelectedIsRunning(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.running = map[string]*tunnel.Desc{"prod": {Name: "prod"}}

	d.cursor = 0
	if d.selectedIsRunning() {
		t.Fatal("dev is not running, selectedIsRunning should be false")
	}
	d.cursor = 1
	if !d.selectedIsRunning() {
		t.Fatal("prod is running, selectedIsRunning should be true")
	}

	empty := newDashboard(&config.Config{}, &tuiPrompter{})
	if empty.selectedIsRunning() {
		t.Fatal("empty dashboard should report false")
	}
}

func TestActionResultUpdatesStatus(t *testing.T) {
	d := dashboardWithRows("dev")

	m, cmd := d.Update(actionResultMsg{verb: "Opened", name: "dev"})
	if cmd == nil {
		t.Fatal("actionResultMsg should return a refresh poll command")
	}
	if got := m.(dashboard).status; got != "Opened dev." {
		t.Fatalf("success status = %q, want %q", got, "Opened dev.")
	}

	m, cmd = d.Update(actionResultMsg{verb: "Opened", name: "dev", err: errStub{}})
	if cmd == nil {
		t.Fatal("actionResultMsg error should still return a refresh poll command")
	}
	if got := m.(dashboard).status; !strings.Contains(got, "Opened dev failed") {
		t.Fatalf("error status = %q, want it to mention the failure", got)
	}
}

func TestCtrlCInModalAbortsPendingAuth(t *testing.T) {
	// One active modal + one queued request, each with its own cap-1 channel.
	req1 := authRequestMsg{questions: []string{"Code:"}, echo: []bool{false}, reply: make(chan authReply, 1)}
	req2 := authRequestMsg{questions: []string{"Code:"}, echo: []bool{false}, reply: make(chan authReply, 1)}
	d := dashboardWithRows("a")
	m, _ := d.Update(req1)
	m, _ = m.(dashboard).Update(req2)
	d = m.(dashboard)
	if d.authModal == nil || len(d.authQueue) != 1 {
		t.Fatalf("setup: modal=%v queue=%d", d.authModal != nil, len(d.authQueue))
	}
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Fatal("ctrl+c in a modal should quit")
	}
	for i, ch := range []chan authReply{req1.reply, req2.reply} {
		select {
		case r := <-ch:
			if !errors.Is(r.err, auth.ErrAborted) {
				t.Fatalf("request %d: got err %v, want ErrAborted", i, r.err)
			}
		default:
			t.Fatalf("request %d: no reply sent on quit", i)
		}
	}
}

// errStub is a minimal error used to exercise the error poll path.
type errStub struct{}

func (errStub) Error() string { return "boom" }

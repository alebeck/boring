package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTestKeyStartsTest(t *testing.T) {
	d := dashboardWithRows("dev")
	m, cmd := d.Update(keyMsg('t'))
	nd := m.(dashboard)
	if cmd == nil {
		t.Fatal("t on a dashboard with rows should return a test command")
	}
	if !strings.HasPrefix(nd.status, "Testing dev") {
		t.Fatalf("status = %q, want a Testing... message", nd.status)
	}

	empty := newDashboard(&config.Config{}, &tuiPrompter{})
	m, cmd = empty.Update(keyMsg('t'))
	if cmd != nil {
		t.Fatal("t with no rows should not return a command")
	}
	if got := m.(dashboard).status; got != "" {
		t.Fatalf("t with no rows should not set a status, got %q", got)
	}
}

func TestTestResultOpensModal(t *testing.T) {
	cases := []struct {
		name   string
		result tunnel.TestResult
	}{
		{"ok", tunnel.TestResult{OK: true, Duration: time.Second}},
		{"error", tunnel.TestResult{OK: false, Err: "handshake failed", Duration: time.Second}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := dashboardWithRows("dev")
			d.status = "Testing dev..."
			res := testResultMsg{name: "dev", result: tc.result}
			m, cmd := d.Update(res)
			nd := m.(dashboard)
			if cmd != nil {
				t.Fatal("testResultMsg should not return a command")
			}
			if nd.testResult == nil {
				t.Fatal("testResultMsg should set d.testResult")
			}
			if nd.status != "" {
				t.Fatalf("testResultMsg should clear the testing status, got %q", nd.status)
			}
		})
	}
}

func TestTestResultModalDismissed(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	res := testResultMsg{name: "dev", result: tunnel.TestResult{OK: true}}
	d.testResult = &res
	wantCursor := d.cursor
	wantRows := len(d.rows)
	m, cmd := d.Update(keyMsg('x'))
	nd := m.(dashboard)
	if nd.testResult != nil {
		t.Fatal("any key should dismiss the test-result modal")
	}
	if isQuit(cmd) {
		t.Fatal("dismissing the modal should not quit")
	}
	if nd.cursor != wantCursor {
		t.Fatalf("dismiss should not move the cursor: got %d, want %d", nd.cursor, wantCursor)
	}
	if len(nd.rows) != wantRows {
		t.Fatalf("dismiss should not change rows: got %d, want %d", len(nd.rows), wantRows)
	}
}

func TestTestResultView(t *testing.T) {
	ok := testResultView(testResultMsg{
		name:   "dev",
		result: tunnel.TestResult{OK: true, Duration: 1500 * time.Millisecond},
	})
	if !strings.Contains(ok, "dev") {
		t.Fatalf("OK view should contain the tunnel name, got:\n%s", ok)
	}
	if !strings.Contains(ok, "connection OK") {
		t.Fatalf("OK view should contain a success indicator, got:\n%s", ok)
	}

	fail := testResultView(testResultMsg{
		name: "dev",
		result: tunnel.TestResult{
			OK:       false,
			Err:      "handshake failed: timeout",
			Duration: 2500 * time.Millisecond,
		},
	})
	if !strings.Contains(fail, "connection failed") {
		t.Fatalf("failure view should contain a failure indicator, got:\n%s", fail)
	}
	if !strings.Contains(fail, "handshake failed: timeout") {
		t.Fatalf("failure view should contain the error text, got:\n%s", fail)
	}
	if !strings.Contains(fail, "failed after") {
		t.Fatalf("failure view should contain a duration indicator, got:\n%s", fail)
	}
}

func TestCtrlCQuitsEverywhere(t *testing.T) {
	ctrlC := tea.KeyMsg{Type: tea.KeyCtrlC}

	t.Run("dashboard", func(t *testing.T) {
		_, cmd := dashboardWithRows("dev").Update(ctrlC)
		if !isQuit(cmd) {
			t.Fatal("ctrl+c should quit from the dashboard")
		}
	})

	t.Run("test-result modal", func(t *testing.T) {
		d := dashboardWithRows("dev")
		res := testResultMsg{name: "dev", result: tunnel.TestResult{OK: true}}
		d.testResult = &res
		_, cmd := d.Update(ctrlC)
		if !isQuit(cmd) {
			t.Fatal("ctrl+c should quit with the test-result modal open")
		}
	})

	t.Run("delete confirmation", func(t *testing.T) {
		d := dashboardWithRows("dev")
		d.confirmDelete = "dev"
		_, cmd := d.Update(ctrlC)
		if !isQuit(cmd) {
			t.Fatal("ctrl+c should quit with the delete confirmation open")
		}
	})

	t.Run("form", func(t *testing.T) {
		d := dashboardWithRows("dev")
		f := newTunnelForm()
		d.form = &f
		_, cmd := d.Update(ctrlC)
		if !isQuit(cmd) {
			t.Fatal("ctrl+c should quit with a form open")
		}
	})
}

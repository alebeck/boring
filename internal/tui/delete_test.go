package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDeleteKeyOpensConfirmation(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.cursor = 1
	m, _ := d.Update(keyMsg('d'))
	if got := m.(dashboard).confirmDelete; got != "prod" {
		t.Fatalf("d should open the confirmation for the selected tunnel, got %q", got)
	}
}

func TestDeleteKeyDoesNothingWithoutRows(t *testing.T) {
	d := newDashboard(&config.Config{}, &tuiPrompter{})
	m, _ := d.Update(keyMsg('d'))
	if got := m.(dashboard).confirmDelete; got != "" {
		t.Fatalf("d with no rows should not open a confirmation, got %q", got)
	}
}

func TestDeleteKeyBlockedForRunningTunnel(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.running = map[string]*tunnel.Desc{"prod": {Name: "prod"}}
	d.cursor = 1
	m, _ := d.Update(keyMsg('d'))
	nd := m.(dashboard)
	if nd.confirmDelete != "" {
		t.Fatalf("d on a running tunnel must not open a confirmation, got %q", nd.confirmDelete)
	}
	if !strings.Contains(nd.status, "prod") || !strings.Contains(nd.status, "Stop") {
		t.Fatalf("d on a running tunnel should set a hint, got %q", nd.status)
	}
}

func TestDeleteConfirmCancel(t *testing.T) {
	for _, c := range []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"n", keyMsg('n')},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := dashboardWithRows("dev", "prod")
			d.confirmDelete = "prod"
			before := len(d.configured)
			m, cmd := d.Update(c.msg)
			nd := m.(dashboard)
			if nd.confirmDelete != "" {
				t.Fatalf("%s should clear the confirmation, got %q", c.name, nd.confirmDelete)
			}
			if len(nd.configured) != before {
				t.Fatalf("%s should not change the configured list", c.name)
			}
			if isQuit(cmd) {
				t.Fatalf("%s should not quit", c.name)
			}
		})
	}
}

func TestDeleteConfirmIgnoresUnrelatedKey(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.confirmDelete = "prod"
	m, _ := d.Update(keyMsg('x'))
	if got := m.(dashboard).confirmDelete; got != "prod" {
		t.Fatalf("an unrelated key should keep the confirmation open, got %q", got)
	}
}

func TestDeleteConfirmRemovesTunnel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".boring.toml")
	saved := config.Path
	config.Path = cfgPath
	t.Cleanup(func() { config.Path = saved })

	// Seed an on-disk config with two tunnels so reload has something to read.
	// Each tunnel needs a forward, otherwise config.Load rejects it.
	seed := &config.Config{Tunnels: []tunnel.Desc{
		{Name: "dev", Host: "example.com", LocalAddress: "9000", RemoteAddress: "localhost:9000"},
		{Name: "prod", Host: "example.com", LocalAddress: "9001", RemoteAddress: "localhost:9001"},
	}}
	if err := config.Save(seed, cfgPath); err != nil {
		t.Fatalf("seeding config failed: %v", err)
	}

	d := dashboardWithRows("dev", "prod")
	d.confirmDelete = "prod"

	m, cmd := d.Update(keyMsg('y'))
	nd := m.(dashboard)

	if nd.confirmDelete != "" {
		t.Fatalf("a confirmed delete should clear the confirmation, got %q", nd.confirmDelete)
	}
	if cmd == nil {
		t.Fatal("a confirmed delete should return a refresh poll command")
	}
	if !strings.Contains(nd.status, "Deleted prod") {
		t.Fatalf("status should report the deletion, got %q", nd.status)
	}
	for _, td := range nd.configured {
		if td.Name == "prod" {
			t.Fatal("prod should be gone from the configured list")
		}
	}
	if len(nd.configured) != 1 {
		t.Fatalf("expected 1 configured tunnel after delete, got %d", len(nd.configured))
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reading config after delete failed: %v", err)
	}
	if strings.Contains(string(data), "prod") {
		t.Fatalf("on-disk config should no longer contain prod:\n%s", data)
	}
	if !strings.Contains(string(data), "dev") {
		t.Fatalf("on-disk config should still contain dev:\n%s", data)
	}
}

func TestDeleteConfirmViaEnter(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".boring.toml")
	saved := config.Path
	config.Path = cfgPath
	t.Cleanup(func() { config.Path = saved })

	seed := &config.Config{Tunnels: []tunnel.Desc{
		{Name: "dev", Host: "example.com", LocalAddress: "9000", RemoteAddress: "localhost:9000"},
		{Name: "prod", Host: "example.com", LocalAddress: "9001", RemoteAddress: "localhost:9001"},
	}}
	if err := config.Save(seed, cfgPath); err != nil {
		t.Fatalf("seeding config failed: %v", err)
	}

	d := dashboardWithRows("dev", "prod")
	d.confirmDelete = "dev"
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.confirmDelete != "" {
		t.Fatalf("enter should confirm the delete, got %q", nd.confirmDelete)
	}
	if !strings.Contains(nd.status, "Deleted dev") {
		t.Fatalf("status should report the deletion, got %q", nd.status)
	}
}

func TestDeleteConfirmView(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.confirmDelete = "prod"
	out := d.View()
	if !strings.Contains(out, "prod") {
		t.Fatalf("delete confirmation view should name the tunnel, got:\n%s", out)
	}
	if !strings.Contains(out, "y / n") {
		t.Fatalf("delete confirmation view should show the y/n hint, got:\n%s", out)
	}
}

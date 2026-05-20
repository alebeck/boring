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

func TestParseOptionalInt(t *testing.T) {
	t.Run("empty yields nil", func(t *testing.T) {
		got, err := parseOptionalInt("", "port")
		if err != nil {
			t.Fatalf("empty should not error, got %v", err)
		}
		if got != nil {
			t.Fatalf("empty should yield nil, got %v", *got)
		}
	})
	t.Run("blank yields nil", func(t *testing.T) {
		got, err := parseOptionalInt("   ", "port")
		if err != nil || got != nil {
			t.Fatalf("blank should yield (nil, nil), got (%v, %v)", got, err)
		}
	})
	t.Run("integer parsed", func(t *testing.T) {
		got, err := parseOptionalInt("22", "port")
		if err != nil {
			t.Fatalf("'22' should not error, got %v", err)
		}
		if got == nil || *got != 22 {
			t.Fatalf("'22' should yield &22, got %v", got)
		}
	})
	t.Run("non-integer errors", func(t *testing.T) {
		_, err := parseOptionalInt("abc", "port")
		if err == nil {
			t.Fatal("'abc' should error")
		}
	})
}

func TestFormFromDescBlanksSocksLabel(t *testing.T) {
	d := tunnel.Desc{
		Name:          "proxy",
		Host:          "example.com",
		Mode:          tunnel.Socks,
		LocalAddress:  tunnel.StringOrInt("1080"),
		RemoteAddress: tunnel.StringOrInt(config.SocksLabel),
	}
	f := formFromDesc(d)
	if got := f.inputs[3].Value(); got != "" {
		t.Fatalf("socks label remote should be blanked, got %q", got)
	}
	if got := f.inputs[2].Value(); got != "1080" {
		t.Fatalf("local should be preserved, got %q", got)
	}
}

func TestFormFromDescBlanksReverseSocksLocal(t *testing.T) {
	d := tunnel.Desc{
		Name:         "rsocks",
		Host:         "example.com",
		Mode:         tunnel.RemoteSocks,
		LocalAddress: tunnel.StringOrInt(config.SocksLabel),
	}
	f := formFromDesc(d)
	if got := f.inputs[2].Value(); got != "" {
		t.Fatalf("reverse-socks label local should be blanked, got %q", got)
	}
}

func TestFormToDescRoundTrip(t *testing.T) {
	f := newTunnelForm()
	f.inputs[0].SetValue("dev")
	f.inputs[1].SetValue("example.com")
	f.inputs[2].SetValue("9000")
	f.inputs[3].SetValue("localhost:9000")
	f.inputs[4].SetValue("alice")
	f.inputs[5].SetValue("~/.ssh/id_ed25519")
	f.inputs[6].SetValue("2222")
	f.inputs[7].SetValue("60")
	f.inputs[8].SetValue("team")
	f.mode = tunnel.Remote

	d, err := f.toDesc()
	if err != nil {
		t.Fatalf("toDesc should not error, got %v", err)
	}
	if d.Name != "dev" || d.Host != "example.com" {
		t.Fatalf("name/host wrong: %q %q", d.Name, d.Host)
	}
	if d.LocalAddress.String() != "9000" || d.RemoteAddress.String() != "localhost:9000" {
		t.Fatalf("addresses wrong: %q %q", d.LocalAddress, d.RemoteAddress)
	}
	if d.Mode != tunnel.Remote {
		t.Fatalf("mode wrong: %v", d.Mode)
	}
	if d.User != "alice" || d.IdentityFile != "~/.ssh/id_ed25519" || d.Group != "team" {
		t.Fatalf("user/identity/group wrong: %q %q %q", d.User, d.IdentityFile, d.Group)
	}
	if d.Port == nil || *d.Port != 2222 {
		t.Fatalf("port wrong: %v", d.Port)
	}
	if d.KeepAlive == nil || *d.KeepAlive != 60 {
		t.Fatalf("keep-alive wrong: %v", d.KeepAlive)
	}
}

func TestFormToDescBadPort(t *testing.T) {
	f := newTunnelForm()
	f.inputs[0].SetValue("dev")
	f.inputs[6].SetValue("nope")
	if _, err := f.toDesc(); err == nil {
		t.Fatal("a non-numeric port should error")
	}
}

func TestFormCycleMode(t *testing.T) {
	f := newTunnelForm()
	f.focus = modeIndex
	want := []tunnel.Mode{
		tunnel.Remote, tunnel.Socks, tunnel.RemoteSocks, tunnel.Local,
	}
	for _, m := range want {
		f, _ = f.update(tea.KeyMsg{Type: tea.KeyRight})
		if f.mode != m {
			t.Fatalf("after right, mode = %v, want %v", f.mode, m)
		}
	}
	// Left wraps backwards.
	f, _ = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	if f.mode != tunnel.RemoteSocks {
		t.Fatalf("after left from local, mode = %v, want socks-remote", f.mode)
	}
}

func TestFormFocusWraps(t *testing.T) {
	f := newTunnelForm()
	if f.focus != 0 {
		t.Fatalf("new form should focus field 0, got %d", f.focus)
	}
	// shift+tab from the first field wraps to the last.
	f, _ = f.update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if f.focus != formFieldCount-1 {
		t.Fatalf("shift+tab from 0 should wrap to %d, got %d", formFieldCount-1, f.focus)
	}
	// tab from the last field wraps back to 0.
	f, _ = f.update(tea.KeyMsg{Type: tea.KeyTab})
	if f.focus != 0 {
		t.Fatalf("tab from last should wrap to 0, got %d", f.focus)
	}
}

func TestFormUpdateActions(t *testing.T) {
	f := newTunnelForm()
	if _, a := f.update(tea.KeyMsg{Type: tea.KeyEnter}); a != formSave {
		t.Fatalf("enter should yield formSave, got %v", a)
	}
	if _, a := f.update(tea.KeyMsg{Type: tea.KeyEsc}); a != formCancel {
		t.Fatalf("esc should yield formCancel, got %v", a)
	}
	if _, a := f.update(tea.KeyMsg{Type: tea.KeyTab}); a != formNone {
		t.Fatalf("tab should yield formNone, got %v", a)
	}
}

func TestFormViewShowsTitleAndHint(t *testing.T) {
	add := newTunnelForm()
	out := add.View()
	if !strings.Contains(out, "Add tunnel") {
		t.Fatalf("add form view should show 'Add tunnel', got:\n%s", out)
	}
	if !strings.Contains(out, "esc cancel") {
		t.Fatalf("form view should show the key hint, got:\n%s", out)
	}

	edit := formFromDesc(tunnel.Desc{Name: "dev", Host: "h"})
	if !strings.Contains(edit.View(), "Edit tunnel: dev") {
		t.Fatalf("edit form view should name the tunnel, got:\n%s", edit.View())
	}
}

func TestDashboardAOpensForm(t *testing.T) {
	d := dashboardWithRows("a")
	m, _ := d.Update(keyMsg('a'))
	nd := m.(dashboard)
	if nd.form == nil {
		t.Fatal("'a' should open a form")
	}
	if nd.form.editing != "" {
		t.Fatalf("'a' should open an add form, editing = %q", nd.form.editing)
	}
}

func TestDashboardEOpensPopulatedForm(t *testing.T) {
	d := dashboardWithRows("dev", "prod")
	d.cursor = 1
	m, _ := d.Update(keyMsg('e'))
	nd := m.(dashboard)
	if nd.form == nil {
		t.Fatal("'e' should open a form for a configured tunnel")
	}
	if nd.form.editing != "prod" {
		t.Fatalf("'e' should edit 'prod', editing = %q", nd.form.editing)
	}
	if got := nd.form.inputs[0].Value(); got != "prod" {
		t.Fatalf("edit form name field should be 'prod', got %q", got)
	}
}

func TestDashboardENoRowsDoesNothing(t *testing.T) {
	d := newDashboard(&config.Config{}, &tuiPrompter{})
	m, _ := d.Update(keyMsg('e'))
	if m.(dashboard).form != nil {
		t.Fatal("'e' with no rows should not open a form")
	}
}

func TestDashboardEOnUnconfiguredTunnelSetsHint(t *testing.T) {
	d := dashboardWithRows("dev")
	// A running, unconfigured tunnel becomes a row but is not in d.configured.
	d.rows = tunnel.Order(d.configured, map[string]*tunnel.Desc{
		"adhoc": {Name: "adhoc"},
	})
	d.cursor = 1 // the adhoc row
	m, _ := d.Update(keyMsg('e'))
	nd := m.(dashboard)
	if nd.form != nil {
		t.Fatal("'e' on an unconfigured tunnel should not open a form")
	}
	if !strings.Contains(nd.status, "not in the config") {
		t.Fatalf("status should explain the tunnel is unconfigured, got %q", nd.status)
	}
}

func TestFormSaveValidationError(t *testing.T) {
	d := dashboardWithRows("dev")
	f := newTunnelForm()
	// An empty name fails config.Validate before any filesystem write.
	d.form = &f
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form == nil {
		t.Fatal("a validation error should keep the form open")
	}
	if nd.form.errMsg == "" {
		t.Fatal("a validation error should set errMsg")
	}
}

func TestFormSaveBadPortKeepsFormOpen(t *testing.T) {
	d := dashboardWithRows("dev")
	f := newTunnelForm()
	f.inputs[0].SetValue("newone")
	f.inputs[6].SetValue("not-a-number")
	d.form = &f
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form == nil || nd.form.errMsg == "" {
		t.Fatal("a bad port should keep the form open with errMsg set")
	}
}

func TestFormSaveHappyPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".boring.toml")
	saved := config.Path
	config.Path = cfgPath
	t.Cleanup(func() { config.Path = saved })

	d := dashboardWithRows("dev")
	f := newTunnelForm()
	f.inputs[0].SetValue("newtun")
	f.inputs[1].SetValue("example.com")
	f.inputs[2].SetValue("8080")
	f.inputs[3].SetValue("localhost:80")
	d.form = &f

	m, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form != nil {
		t.Fatalf("a successful save should close the form, errMsg = %q", nd.form.errMsg)
	}
	if cmd == nil {
		t.Fatal("a successful save should return a refresh poll command")
	}
	if !strings.Contains(nd.status, "Saved newtun") {
		t.Fatalf("status should report the save, got %q", nd.status)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file should exist after save: %v", err)
	}
}

func TestFormEscClosesForm(t *testing.T) {
	d := dashboardWithRows("dev")
	f := newTunnelForm()
	d.form = &f
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.(dashboard).form != nil {
		t.Fatal("esc should close the form")
	}
}

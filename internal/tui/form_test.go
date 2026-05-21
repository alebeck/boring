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

func TestNewTunnelFormStructure(t *testing.T) {
	f := newTunnelForm()
	if len(f.conn) != connFieldCount {
		t.Fatalf("new form should have %d connection fields, got %d",
			connFieldCount, len(f.conn))
	}
	if len(f.forwards) != 1 {
		t.Fatalf("new form should start with exactly one forward, got %d",
			len(f.forwards))
	}
	if f.forwards[0].mode != tunnel.Local {
		t.Fatalf("a new forward should default to local mode, got %v",
			f.forwards[0].mode)
	}
	for _, v := range []string{
		f.forwards[0].name.Value(),
		f.forwards[0].local.Value(),
		f.forwards[0].remote.Value(),
	} {
		if v != "" {
			t.Fatalf("a new forward should be empty, got %q", v)
		}
	}
	if f.focus != 0 {
		t.Fatalf("new form should focus field 0, got %d", f.focus)
	}
}

func TestFormFromDescMultiForward(t *testing.T) {
	port := 2222
	d := tunnel.Desc{
		Name:         "prod",
		Host:         "bastion",
		User:         "deploy",
		IdentityFile: "~/.ssh/id_ed25519",
		Port:         &port,
		Group:        "team",
		Forwards: []tunnel.Forward{
			{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432"},
			{LocalAddress: "6379", RemoteAddress: "redis:6379", Mode: tunnel.Remote},
		},
	}
	f := formFromDesc(d)
	if f.editing != "prod" {
		t.Fatalf("editing should be 'prod', got %q", f.editing)
	}
	if got := f.conn[0].Value(); got != "prod" {
		t.Fatalf("conn name should be 'prod', got %q", got)
	}
	if got := f.conn[1].Value(); got != "bastion" {
		t.Fatalf("conn host should be 'bastion', got %q", got)
	}
	if got := f.conn[2].Value(); got != "deploy" {
		t.Fatalf("conn user should be 'deploy', got %q", got)
	}
	if got := f.conn[4].Value(); got != "2222" {
		t.Fatalf("conn port should be '2222', got %q", got)
	}
	if got := f.conn[6].Value(); got != "team" {
		t.Fatalf("conn group should be 'team', got %q", got)
	}
	if len(f.forwards) != 2 {
		t.Fatalf("form should load 2 forwards, got %d", len(f.forwards))
	}
	if got := f.forwards[0].name.Value(); got != "db" {
		t.Fatalf("forward 0 name should be 'db', got %q", got)
	}
	if got := f.forwards[0].local.Value(); got != "5432" {
		t.Fatalf("forward 0 local should be '5432', got %q", got)
	}
	if got := f.forwards[1].remote.Value(); got != "redis:6379" {
		t.Fatalf("forward 1 remote should be 'redis:6379', got %q", got)
	}
	if f.forwards[1].mode != tunnel.Remote {
		t.Fatalf("forward 1 mode should be remote, got %v", f.forwards[1].mode)
	}
}

func TestFormFromDescBlanksSocksLabel(t *testing.T) {
	d := tunnel.Desc{
		Name: "proxy",
		Host: "example.com",
		Forwards: []tunnel.Forward{{
			Mode:          tunnel.Socks,
			LocalAddress:  tunnel.StringOrInt("1080"),
			RemoteAddress: tunnel.StringOrInt(config.SocksLabel),
		}},
	}
	f := formFromDesc(d)
	if got := f.forwards[0].remote.Value(); got != "" {
		t.Fatalf("socks label remote should be blanked, got %q", got)
	}
	if got := f.forwards[0].local.Value(); got != "1080" {
		t.Fatalf("local should be preserved, got %q", got)
	}
}

func TestFormFromDescBlanksReverseSocksLocal(t *testing.T) {
	d := tunnel.Desc{
		Name: "rsocks",
		Host: "example.com",
		Forwards: []tunnel.Forward{{
			Mode:         tunnel.RemoteSocks,
			LocalAddress: tunnel.StringOrInt(config.SocksLabel),
		}},
	}
	f := formFromDesc(d)
	if got := f.forwards[0].local.Value(); got != "" {
		t.Fatalf("reverse-socks label local should be blanked, got %q", got)
	}
}

func TestFormToDescRoundTrip(t *testing.T) {
	f := newTunnelForm()
	f.conn[0].SetValue("dev")
	f.conn[1].SetValue("example.com")
	f.conn[2].SetValue("alice")
	f.conn[3].SetValue("~/.ssh/id_ed25519")
	f.conn[4].SetValue("2222")
	f.conn[5].SetValue("60")
	f.conn[6].SetValue("team")
	f.forwards[0].local.SetValue("9000")
	f.forwards[0].remote.SetValue("localhost:9000")
	f.forwards[0].mode = tunnel.Remote

	d, err := f.toDesc()
	if err != nil {
		t.Fatalf("toDesc should not error, got %v", err)
	}
	if d.Name != "dev" || d.Host != "example.com" {
		t.Fatalf("name/host wrong: %q %q", d.Name, d.Host)
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
	if len(d.Forwards) != 1 {
		t.Fatalf("should build one forward, got %d", len(d.Forwards))
	}
	if d.Forwards[0].LocalAddress.String() != "9000" ||
		d.Forwards[0].RemoteAddress.String() != "localhost:9000" {
		t.Fatalf("forward addresses wrong: %q %q",
			d.Forwards[0].LocalAddress, d.Forwards[0].RemoteAddress)
	}
	if d.Forwards[0].Mode != tunnel.Remote {
		t.Fatalf("forward mode wrong: %v", d.Forwards[0].Mode)
	}
}

func TestFormToDescMultiForward(t *testing.T) {
	f := newTunnelForm()
	f.conn[0].SetValue("prod")
	f.conn[1].SetValue("bastion")
	f.forwards[0].name.SetValue("db")
	f.forwards[0].local.SetValue("5432")
	f.forwards[0].remote.SetValue("db:5432")
	f = f.addForward()
	f.forwards[1].local.SetValue("6379")
	f.forwards[1].remote.SetValue("redis:6379")

	d, err := f.toDesc()
	if err != nil {
		t.Fatalf("toDesc should not error, got %v", err)
	}
	if len(d.Forwards) != 2 {
		t.Fatalf("should build two forwards, got %d", len(d.Forwards))
	}
	if d.Forwards[0].Name != "db" || d.Forwards[0].LocalAddress.String() != "5432" {
		t.Fatalf("forward 0 wrong: %+v", d.Forwards[0])
	}
	if d.Forwards[1].LocalAddress.String() != "6379" ||
		d.Forwards[1].RemoteAddress.String() != "redis:6379" {
		t.Fatalf("forward 1 wrong: %+v", d.Forwards[1])
	}
}

func TestFormToDescBadPort(t *testing.T) {
	f := newTunnelForm()
	f.conn[0].SetValue("dev")
	f.conn[4].SetValue("nope")
	if _, err := f.toDesc(); err == nil {
		t.Fatal("a non-numeric port should error")
	}
}

func TestFormCycleMode(t *testing.T) {
	f := newTunnelForm()
	// Focus the first forward's Mode sub-field.
	f.focus = connFieldCount + subMode
	if !f.onModeField() {
		t.Fatal("focus should be on the Mode sub-field")
	}
	want := []tunnel.Mode{
		tunnel.Remote, tunnel.Socks, tunnel.RemoteSocks, tunnel.Local,
	}
	for _, m := range want {
		f, _ = f.update(tea.KeyMsg{Type: tea.KeyRight})
		if f.forwards[0].mode != m {
			t.Fatalf("after right, mode = %v, want %v", f.forwards[0].mode, m)
		}
	}
	// Left wraps backwards.
	f, _ = f.update(tea.KeyMsg{Type: tea.KeyLeft})
	if f.forwards[0].mode != tunnel.RemoteSocks {
		t.Fatalf("after left from local, mode = %v, want socks-remote",
			f.forwards[0].mode)
	}
}

func TestFormFocusWalksAllFields(t *testing.T) {
	f := newTunnelForm()
	// One forward → connFieldCount + 4 focusable fields.
	if got := f.fieldCount(); got != connFieldCount+subFieldsPerForward {
		t.Fatalf("fieldCount with one forward = %d, want %d",
			got, connFieldCount+subFieldsPerForward)
	}
	// Tab through every field; focus should land back on 0.
	for i := 0; i < f.fieldCount(); i++ {
		f, _ = f.update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if f.focus != 0 {
		t.Fatalf("tabbing a full cycle should return to field 0, got %d", f.focus)
	}
	// shift+tab from field 0 wraps to the last field.
	f, _ = f.update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if f.focus != f.fieldCount()-1 {
		t.Fatalf("shift+tab from 0 should wrap to %d, got %d",
			f.fieldCount()-1, f.focus)
	}
	// That last field is the only forward's Mode sub-field.
	if !f.onModeField() {
		t.Fatal("the last focusable field should be a forward's Mode selector")
	}
}

func TestFormFocusEntersForwardSubFields(t *testing.T) {
	f := newTunnelForm()
	// Tab through the connection fields onto the first forward sub-field.
	for i := 0; i < connFieldCount; i++ {
		f, _ = f.update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if f.onConnField() {
		t.Fatal("after the connection fields, focus should be in a forward")
	}
	if f.focusedForward() != 0 || f.focusedSub() != subName {
		t.Fatalf("first forward sub-field should be (0, subName), got (%d, %d)",
			f.focusedForward(), f.focusedSub())
	}
}

func TestFormAddForwardAppends(t *testing.T) {
	f := newTunnelForm()
	f = f.addForward()
	if len(f.forwards) != 2 {
		t.Fatalf("add-forward should append a forward, got %d", len(f.forwards))
	}
	// Focus should move to the new forward's Name sub-field.
	if f.onConnField() || f.focusedForward() != 1 || f.focusedSub() != subName {
		t.Fatalf("after add, focus should be on forward 1 Name, got conn=%v fwd=%d sub=%d",
			f.onConnField(), f.focusedForward(), f.focusedSub())
	}
}

func TestFormAddForwardViaKey(t *testing.T) {
	f := newTunnelForm()
	f, action := f.update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if action != formNone {
		t.Fatalf("ctrl+f should be consumed, got action %v", action)
	}
	if len(f.forwards) != 2 {
		t.Fatalf("ctrl+f should add a forward, got %d", len(f.forwards))
	}
}

func TestFormAddForwardInsertsAfterFocused(t *testing.T) {
	f := newTunnelForm()
	f = f.addForward() // two forwards, focus on forward 1
	f = f.addForward() // insert after forward 1 → three forwards, focus on 2
	if len(f.forwards) != 3 {
		t.Fatalf("expected 3 forwards, got %d", len(f.forwards))
	}
	if f.focusedForward() != 2 {
		t.Fatalf("new forward should be inserted after the focused one, got %d",
			f.focusedForward())
	}
}

func TestFormRemoveForward(t *testing.T) {
	f := newTunnelForm()
	f = f.addForward() // two forwards, focus on forward 1
	f.forwards[1].local.SetValue("marker")
	f = f.removeForward()
	if len(f.forwards) != 1 {
		t.Fatalf("remove-forward should drop the focused forward, got %d",
			len(f.forwards))
	}
	if f.forwards[0].local.Value() == "marker" {
		t.Fatal("remove-forward should drop the focused (second) forward, not the first")
	}
}

func TestFormRemoveLastForwardPrevented(t *testing.T) {
	f := newTunnelForm()
	// Focus a forward sub-field so removeForward applies to it.
	f.focus = connFieldCount + subName
	f = f.removeForward()
	if len(f.forwards) != 1 {
		t.Fatalf("removing the last forward must be prevented, got %d forwards",
			len(f.forwards))
	}
	if f.errMsg == "" {
		t.Fatal("removing the last forward should set errMsg")
	}
}

func TestFormRemoveForwardOnConnFieldIsNoOp(t *testing.T) {
	f := newTunnelForm()
	f = f.addForward() // two forwards
	f.focus = 0        // a connection field
	f = f.removeForward()
	if len(f.forwards) != 2 {
		t.Fatalf("remove-forward on a connection field should be a no-op, got %d",
			len(f.forwards))
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

func TestFormView(t *testing.T) {
	add := newTunnelForm()
	view := add.View()
	if !strings.Contains(view, "Add tunnel") {
		t.Fatalf("add form view should show 'Add tunnel', got:\n%s", view)
	}
	if !strings.Contains(view, "Forward 1") {
		t.Fatalf("add form view should show the forward heading, got:\n%s", view)
	}

	edit := formFromDesc(tunnel.Desc{
		Name:     "dev",
		Host:     "h",
		Forwards: []tunnel.Forward{{LocalAddress: "1", RemoteAddress: "2"}},
	})
	if !strings.Contains(edit.View(), "Edit tunnel: dev") {
		t.Fatalf("edit form view should name the tunnel, got:\n%s", edit.View())
	}

	// The form's key hint lives in the dashboard footer, not the form body.
	d := dashboardWithRows("a")
	d.form = &add
	if !strings.Contains(d.View(), "esc cancel") {
		t.Fatalf("dashboard footer should show the form key hint, got:\n%s", d.View())
	}
	if !strings.Contains(d.View(), "add forward") {
		t.Fatalf("form footer hint should mention add forward, got:\n%s", d.View())
	}
}

func TestFormViewMultiForward(t *testing.T) {
	f := newTunnelForm()
	f = f.addForward()
	view := f.View()
	if !strings.Contains(view, "Forward 1") || !strings.Contains(view, "Forward 2") {
		t.Fatalf("a two-forward form should show both headings, got:\n%s", view)
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
	if got := nd.form.conn[0].Value(); got != "prod" {
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
	f.conn[0].SetValue("newone")
	f.conn[4].SetValue("not-a-number")
	d.form = &f
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form == nil || nd.form.errMsg == "" {
		t.Fatal("a bad port should keep the form open with errMsg set")
	}
}

func TestFormSaveInvalidForwardKeepsFormOpen(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".boring.toml")
	saved := config.Path
	config.Path = cfgPath
	t.Cleanup(func() { config.Path = saved })

	d := dashboardWithRows("dev")
	f := newTunnelForm()
	f.conn[0].SetValue("newtun")
	f.conn[1].SetValue("example.com")
	// A local-mode forward with no addresses fails config.Validate.
	d.form = &f
	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form == nil {
		t.Fatal("an invalid forward should keep the form open")
	}
	if nd.form.errMsg == "" {
		t.Fatal("an invalid forward should set errMsg")
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
	f.conn[0].SetValue("newtun")
	f.conn[1].SetValue("example.com")
	f.forwards[0].local.SetValue("8080")
	f.forwards[0].remote.SetValue("localhost:80")
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

func TestFormSaveMultiForwardWritesBlocks(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".boring.toml")
	saved := config.Path
	config.Path = cfgPath
	t.Cleanup(func() { config.Path = saved })

	d := dashboardWithRows("dev")
	f := newTunnelForm()
	f.conn[0].SetValue("prod")
	f.conn[1].SetValue("bastion")
	f.forwards[0].name.SetValue("db")
	f.forwards[0].local.SetValue("5432")
	f.forwards[0].remote.SetValue("db:5432")
	f = f.addForward()
	f.forwards[1].local.SetValue("6379")
	f.forwards[1].remote.SetValue("redis:6379")
	d.form = &f

	m, _ := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)
	if nd.form != nil {
		t.Fatalf("a valid multi-forward save should close the form, errMsg = %q",
			nd.form.errMsg)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config file should exist after save: %v", err)
	}
	if !strings.Contains(string(data), "[[tunnels.forward]]") {
		t.Fatalf("a multi-forward save should write [[tunnels.forward]] blocks,"+
			" got:\n%s", data)
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

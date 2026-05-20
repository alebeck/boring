package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

const pollInterval = 1500 * time.Millisecond

// daemonUnavailablePrefix marks status-bar messages caused by a failed poll,
// so a later successful poll can clear them.
const daemonUnavailablePrefix = "daemon unavailable"

// dashboard is the root TUI model: a live, navigable list of tunnels.
type dashboard struct {
	configured []tunnel.Desc           // tunnels from the config file, fixed order
	rows       []*tunnel.Desc          // merged configured + running, what is displayed
	running    map[string]*tunnel.Desc // currently running tunnels, by name
	cursor     int
	status     string // status-bar message (errors, hints)
	showHelp   bool
	width      int
	height     int
}

// newDashboard builds the initial dashboard from the loaded config.
func newDashboard(conf *config.Config) dashboard {
	d := dashboard{configured: conf.Tunnels}
	d.rows = tunnel.Order(d.configured, nil)
	return d
}

func (d dashboard) Init() tea.Cmd {
	return pollTunnels()
}

func (d dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
		return d, nil
	case tunnelsMsg:
		return d.handleTunnels(msg)
	case pollTick:
		return d, pollTunnels()
	case actionResultMsg:
		return d.handleActionResult(msg)
	case tea.KeyMsg:
		return d.handleKey(msg)
	}
	return d, nil
}

// handleActionResult records an open/close outcome and refreshes the table.
func (d dashboard) handleActionResult(msg actionResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		d.status = fmt.Sprintf("%s %s failed: %v", msg.verb, msg.name, msg.err)
	} else {
		d.status = fmt.Sprintf("%s %s.", msg.verb, msg.name)
	}
	return d, pollTunnels()
}

// handleTunnels merges a poll result and schedules the next poll.
func (d dashboard) handleTunnels(msg tunnelsMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		d.status = daemonUnavailablePrefix + ": " + msg.err.Error()
	} else {
		d.running = msg.running
		d.rows = tunnel.Order(d.configured, msg.running)
		if strings.HasPrefix(d.status, daemonUnavailablePrefix) {
			d.status = ""
		}
	}
	d.clampCursor()
	return d, tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTick{} })
}

// handleKey processes a keypress.
func (d dashboard) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case keyQuit, keyCtrlC:
		return d, tea.Quit
	case keyHelp:
		d.showHelp = !d.showHelp
		return d, nil
	case keyUp, keyVimUp:
		if d.cursor > 0 {
			d.cursor--
		}
		return d, nil
	case keyDown, keyVimDown:
		if d.cursor < len(d.rows)-1 {
			d.cursor++
		}
		return d, nil
	case keyEnter, keySpace:
		return d.toggleSelected()
	}
	return d, nil
}

// toggleSelected opens the selected tunnel if it is not running, or closes it
// if it is. It does nothing when there are no rows.
func (d dashboard) toggleSelected() (tea.Model, tea.Cmd) {
	if len(d.rows) == 0 {
		return d, nil
	}
	name := d.rows[d.cursor].Name
	if d.selectedIsRunning() {
		d.status = "Closing " + name + "..."
		return d, closeTunnelCmd(name)
	}
	d.status = "Opening " + name + "..."
	return d, openTunnelCmd(*d.rows[d.cursor], tuiOpenPrompter())
}

// selectedIsRunning reports whether the tunnel under the cursor is running.
func (d dashboard) selectedIsRunning() bool {
	if len(d.rows) == 0 {
		return false
	}
	_, ok := d.running[d.rows[d.cursor].Name]
	return ok
}

// tuiOpenPrompter returns the prompter used for TUI-initiated opens. It is a
// placeholder that fails any interactive auth request with a clear message;
// pubkey/agent tunnels never invoke it and open fine. A real modal-backed
// prompter replaces this in a later task.
func tuiOpenPrompter() auth.Prompter {
	return auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
		return nil, errors.New("interactive authentication is not available in the TUI yet")
	})
}

// clampCursor keeps the cursor within the current row range.
func (d *dashboard) clampCursor() {
	if d.cursor >= len(d.rows) {
		d.cursor = len(d.rows) - 1
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
}

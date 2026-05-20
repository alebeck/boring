package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	"github.com/charmbracelet/bubbles/textinput"
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
	prompter   *tuiPrompter     // relays interactive auth prompts to the modal
	authModal  *authModal       // active auth modal, nil when none is shown
	authQueue  []authRequestMsg // auth requests waiting for the active modal to finish
}

// newDashboard builds the initial dashboard from the loaded config.
func newDashboard(conf *config.Config, prompter *tuiPrompter) dashboard {
	d := dashboard{configured: conf.Tunnels, prompter: prompter}
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
	case authRequestMsg:
		return d.handleAuthRequest(msg)
	case tea.KeyMsg:
		if d.authModal != nil {
			return d.updateAuthModal(msg)
		}
		return d.handleKey(msg)
	}
	return d, nil
}

// handleAuthRequest shows a new auth modal, or queues the request behind the
// one already on screen so modals are answered one at a time.
func (d dashboard) handleAuthRequest(msg authRequestMsg) (tea.Model, tea.Cmd) {
	if d.authModal == nil {
		am := newAuthModal(msg)
		d.authModal = &am
	} else {
		d.authQueue = append(d.authQueue, msg)
	}
	return d, textinput.Blink
}

// updateAuthModal routes a keypress to the active auth modal: ctrl+c aborts
// every pending request and quits, esc aborts the active request, enter submits
// the current answer (and advances to the next question or resolves the
// request), any other key edits the text input.
func (d dashboard) updateAuthModal(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyCtrlC:
		d.abortAllAuth()
		return d, tea.Quit
	case tea.KeyEsc:
		d.authModal.req.reply <- authReply{err: auth.ErrAborted}
		d.advanceAuthQueue()
		return d, textinput.Blink
	case tea.KeyEnter:
		return d.submitAuthAnswer()
	default:
		m := *d.authModal
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		d.authModal = &m
		return d, cmd
	}
}

// submitAuthAnswer records the current answer; once every question is answered
// it sends the reply and advances the queue, otherwise it moves to the next
// question on the same modal.
func (d dashboard) submitAuthAnswer() (tea.Model, tea.Cmd) {
	m := *d.authModal
	m.answers = append(m.answers, m.input.Value())
	m.idx++
	if m.idx >= len(m.req.questions) {
		m.req.reply <- authReply{answers: m.answers}
		d.authModal = &m
		d.advanceAuthQueue()
		return d, textinput.Blink
	}
	m.configureInput()
	d.authModal = &m
	return d, textinput.Blink
}

// abortAllAuth resolves every pending auth request — the open modal and all
// queued ones — with ErrAborted, so command goroutines blocked in
// tuiPrompter.Prompt unblock cleanly. Each reply channel is cap-1 and
// unreplied here, so the sends never block.
func (d dashboard) abortAllAuth() {
	if d.authModal != nil {
		d.authModal.req.reply <- authReply{err: auth.ErrAborted}
	}
	for _, req := range d.authQueue {
		req.reply <- authReply{err: auth.ErrAborted}
	}
}

// advanceAuthQueue closes the finished modal and opens the next queued request,
// if any.
func (d *dashboard) advanceAuthQueue() {
	if len(d.authQueue) > 0 {
		next := newAuthModal(d.authQueue[0])
		d.authQueue = d.authQueue[1:]
		d.authModal = &next
		return
	}
	d.authModal = nil
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
		d.abortAllAuth()
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
	return d, openTunnelCmd(*d.rows[d.cursor], d.prompter)
}

// selectedIsRunning reports whether the tunnel under the cursor is running.
func (d dashboard) selectedIsRunning() bool {
	if len(d.rows) == 0 {
		return false
	}
	_, ok := d.running[d.rows[d.cursor].Name]
	return ok
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

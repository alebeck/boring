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
	keepAlive  *int                    // global keep-alive, preserved across saves
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
	form       *tunnelForm      // active add/edit form, nil when none is shown
}

// newDashboard builds the initial dashboard from the loaded config.
func newDashboard(conf *config.Config, prompter *tuiPrompter) dashboard {
	d := dashboard{
		configured: conf.Tunnels,
		keepAlive:  conf.KeepAlive,
		prompter:   prompter,
	}
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
		return d.routeKey(msg)
	}
	return d, nil
}

// routeKey dispatches a keypress: the auth modal takes precedence, then the
// add/edit form, otherwise the dashboard's own key handling. In practice at
// most one of {modal, form} is active.
func (d dashboard) routeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if d.authModal != nil {
		return d.updateAuthModal(msg)
	}
	if d.form != nil {
		return d.routeFormKey(msg)
	}
	return d.handleKey(msg)
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
	case keyAdd:
		return d.openAddForm()
	case keyEdit:
		return d.openEditForm()
	}
	return d, nil
}

// openAddForm shows an empty form for a new tunnel.
func (d dashboard) openAddForm() (tea.Model, tea.Cmd) {
	f := newTunnelForm()
	d.form = &f
	return d, nil
}

// openEditForm shows a form populated from the selected configured tunnel. It
// does nothing when there are no rows, and sets a hint when the selected tunnel
// is running but not present in the config.
func (d dashboard) openEditForm() (tea.Model, tea.Cmd) {
	if len(d.rows) == 0 {
		return d, nil
	}
	name := d.rows[d.cursor].Name
	for i := range d.configured {
		if d.configured[i].Name == name {
			f := formFromDesc(d.configured[i])
			d.form = &f
			return d, nil
		}
	}
	d.status = name + " is not in the config"
	return d, nil
}

// routeFormKey routes a keypress into the active form and acts on the result.
func (d dashboard) routeFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	form, action := d.form.update(msg)
	switch action {
	case formCancel:
		d.form = nil
		return d, nil
	case formSave:
		d.form = &form
		return d.saveForm()
	default:
		d.form = &form
		return d, nil
	}
}

// saveForm runs the save flow for the active form: build a Desc, validate the
// resulting tunnel list, write the config, and reload it. Any error keeps the
// form open with errMsg set; a success closes the form and refreshes the list.
func (d dashboard) saveForm() (tea.Model, tea.Cmd) {
	desc, err := d.form.toDesc()
	if err != nil {
		d.setFormError(err)
		return d, nil
	}
	newList := d.tunnelListWith(desc)
	if err := config.Validate(newList); err != nil {
		d.setFormError(err)
		return d, nil
	}
	cfg := &config.Config{Tunnels: newList, KeepAlive: d.keepAlive}
	if err := config.Save(cfg, config.Path); err != nil {
		d.setFormError(err)
		return d, nil
	}
	d.applySavedConfig(desc.Name)
	return d, pollTunnels()
}

// setFormError records err on the form so the save flow can show it inline.
func (d *dashboard) setFormError(err error) {
	d.form.errMsg = err.Error()
}

// tunnelListWith returns a copy of the configured tunnels with desc applied:
// the matching entry replaced when editing, desc appended when adding.
func (d dashboard) tunnelListWith(desc tunnel.Desc) []tunnel.Desc {
	newList := make([]tunnel.Desc, 0, len(d.configured)+1)
	newList = append(newList, d.configured...)
	if d.form.editing != "" {
		for i := range newList {
			if newList[i].Name == d.form.editing {
				newList[i] = desc
				return newList
			}
		}
	}
	return append(newList, desc)
}

// applySavedConfig reloads the config after a successful save, closes the form,
// and reports the saved tunnel in the status bar.
func (d *dashboard) applySavedConfig(name string) {
	if conf, err := config.Load(); err == nil {
		d.configured = conf.Tunnels
		d.keepAlive = conf.KeepAlive
		d.rows = tunnel.Order(d.configured, d.running)
	}
	d.status = "Saved " + name + "."
	d.form = nil
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

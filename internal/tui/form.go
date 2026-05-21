package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/tunnel"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formAction is the outcome of routing a key into a tunnelForm.
type formAction int

const (
	// formNone means the key was consumed and the form stays open.
	formNone formAction = iota
	// formCancel means the user cancelled the form (esc).
	formCancel
	// formSave means the user requested a save (enter); the dashboard runs
	// the save flow.
	formSave
)

// connFieldCount is the number of connection-level text fields, in tab order:
// Name, Host, User, Identity, Port, KeepAlive, Group. Mode is not here — it is
// per-forward.
const connFieldCount = 7

// subFieldsPerForward is the number of focusable sub-fields in one forward:
// Name, Local, Remote, Mode (Mode is a selector, the other three are inputs).
const subFieldsPerForward = 4

// Sub-field indices within a forward.
const (
	subName = iota
	subLocal
	subRemote
	subMode
)

// Connection-field indices within f.conn, in tab order. These must mirror the
// order of connLabels exactly.
const (
	connName = iota
	connHost
	connUser
	connIdentity
	connPort
	connKeepAlive
	connGroup
)

// keyAddForward appends a new forward; keyRemoveForward drops the focused one.
// Both are control chords so they cannot collide with text entry, and neither
// is ctrl+c (quit).
const (
	keyAddForward    = "ctrl+f"
	keyRemoveForward = "ctrl+x"
)

// formModes lists the modes the selector cycles through, in order.
var formModes = []tunnel.Mode{
	tunnel.Local, tunnel.Remote, tunnel.Socks, tunnel.RemoteSocks,
}

// connLabels are the connection-field labels, in tab order.
var connLabels = []string{
	"Name", "Host", "User", "Identity", "Port", "KeepAlive", "Group",
}

// subLabels are the per-forward sub-field labels, in tab order.
var subLabels = []string{"Name", "Local", "Remote", "Mode"}

// forwardFields holds one forward's editable sub-fields.
type forwardFields struct {
	name, local, remote textinput.Model
	mode                tunnel.Mode
}

// newForwardFields builds an empty forward with default (Local) mode.
func newForwardFields() forwardFields {
	return forwardFields{
		name:   textinput.New(),
		local:  textinput.New(),
		remote: textinput.New(),
		mode:   tunnel.Local,
	}
}

// forwardFromDesc builds a forward sub-editor entry from a tunnel.Forward. A
// local/remote value equal to config.SocksLabel is shown empty: that label is
// display-only and must not be presented back to the user or re-saved.
func forwardFromDesc(fwd tunnel.Forward) forwardFields {
	f := newForwardFields()
	f.name.SetValue(fwd.Name)
	f.local.SetValue(blankSocksLabel(fwd.LocalAddress.String()))
	f.remote.SetValue(blankSocksLabel(fwd.RemoteAddress.String()))
	f.mode = fwd.Mode
	return f
}

// input returns a pointer to the sub-field's textinput for the given sub
// index. It must only be called for sub indices subName/subLocal/subRemote; a
// non-text sub-field (subMode) panics so a future caller that forgets to gate
// on subMode fails loudly instead of silently editing the wrong field.
func (ff *forwardFields) input(sub int) *textinput.Model {
	switch sub {
	case subName:
		return &ff.name
	case subLocal:
		return &ff.local
	case subRemote:
		return &ff.remote
	default:
		panic(fmt.Sprintf("input: not a text sub-field: %d", sub))
	}
}

// tunnelForm collects the fields of one tunnel.Desc for adding or editing: a
// fixed set of connection fields followed by a forwards sub-editor of one or
// more forwards.
type tunnelForm struct {
	conn     []textinput.Model // connFieldCount connection fields, in order
	forwards []forwardFields   // always length >= 1
	focus    int               // flat index over every focusable field
	editing  string            // name of the tunnel being edited; "" when adding
	errMsg   string
}

// newTunnelForm builds an empty form for a new tunnel: the connection fields
// blank and exactly one empty forward.
func newTunnelForm() tunnelForm {
	f := tunnelForm{
		conn:     make([]textinput.Model, connFieldCount),
		forwards: []forwardFields{newForwardFields()},
	}
	for i := range f.conn {
		f.conn[i] = textinput.New()
	}
	f.refocus()
	return f
}

// formFromDesc builds a form populated from d for editing: the connection
// fields from d, and one forward sub-editor entry per d.Forwards element.
func formFromDesc(d tunnel.Desc) tunnelForm {
	f := newTunnelForm()
	f.editing = d.Name
	f.conn[connName].SetValue(d.Name)
	f.conn[connHost].SetValue(d.Host)
	f.conn[connUser].SetValue(d.User)
	f.conn[connIdentity].SetValue(d.IdentityFile)
	f.conn[connPort].SetValue(intToString(d.Port))
	f.conn[connKeepAlive].SetValue(intToString(d.KeepAlive))
	f.conn[connGroup].SetValue(d.Group)
	if len(d.Forwards) > 0 {
		f.forwards = make([]forwardFields, len(d.Forwards))
		for i, fwd := range d.Forwards {
			f.forwards[i] = forwardFromDesc(fwd)
		}
	}
	f.refocus()
	return f
}

// blankSocksLabel returns "" when s is the display-only socks label, else s.
func blankSocksLabel(s string) string {
	if s == config.SocksLabel {
		return ""
	}
	return s
}

// intToString renders an optional int: "" when nil, else the decimal number.
func intToString(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// parseOptionalInt parses an optional integer field: an empty string yields a
// nil pointer, a valid integer yields a pointer to it, anything else an error.
func parseOptionalInt(s, field string) (*int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", field)
	}
	return &n, nil
}

// fieldCount is the total number of focusable fields: every connection field
// plus subFieldsPerForward for each forward.
func (f tunnelForm) fieldCount() int {
	return connFieldCount + len(f.forwards)*subFieldsPerForward
}

// onConnField reports whether the current focus is a connection field.
func (f tunnelForm) onConnField() bool {
	return f.focus < connFieldCount
}

// focusedForward returns the index of the forward the focus is on. It must
// only be called when onConnField is false.
func (f tunnelForm) focusedForward() int {
	return (f.focus - connFieldCount) / subFieldsPerForward
}

// focusedSub returns the sub-field index (subName..subMode) the focus is on.
// It must only be called when onConnField is false.
func (f tunnelForm) focusedSub() int {
	return (f.focus - connFieldCount) % subFieldsPerForward
}

// onModeField reports whether the focus is on a forward's Mode selector.
func (f tunnelForm) onModeField() bool {
	return !f.onConnField() && f.focusedSub() == subMode
}

// focusedInput returns the textinput the focus currently controls, or nil when
// the focus is on a Mode selector (which is not a textinput).
func (f *tunnelForm) focusedInput() *textinput.Model {
	if f.onConnField() {
		return &f.conn[f.focus]
	}
	if f.focusedSub() == subMode {
		return nil
	}
	return f.forwards[f.focusedForward()].input(f.focusedSub())
}

// refocus blurs every textinput and focuses only the one under the current
// focus index. It is the single place that keeps textinput focus state in sync
// with f.focus, called after construction and any focus/structure change.
func (f *tunnelForm) refocus() {
	for i := range f.conn {
		f.conn[i].Blur()
	}
	for i := range f.forwards {
		f.forwards[i].name.Blur()
		f.forwards[i].local.Blur()
		f.forwards[i].remote.Blur()
	}
	if in := f.focusedInput(); in != nil {
		in.Focus()
	}
}

// update routes a key into the form and returns the new form and the action
// the dashboard should take.
func (f tunnelForm) update(k tea.KeyMsg) (tunnelForm, formAction) {
	switch k.String() {
	case keyEnter:
		return f, formSave
	case keyEsc:
		return f, formCancel
	case keyTab, keyDown:
		return f.moveFocus(1), formNone
	case keyShiftTab, keyUp:
		return f.moveFocus(-1), formNone
	case keyAddForward:
		return f.addForward(), formNone
	case keyRemoveForward:
		return f.removeForward(), formNone
	case keyLeft:
		if f.onModeField() {
			return f.cycleMode(-1), formNone
		}
	case keyRight:
		if f.onModeField() {
			return f.cycleMode(1), formNone
		}
	}
	return f.forwardToInput(k), formNone
}

// moveFocus shifts focus by delta (wrapping) and re-focuses the active input.
func (f tunnelForm) moveFocus(delta int) tunnelForm {
	n := f.fieldCount()
	f.focus = (f.focus + delta + n) % n
	f.refocus()
	return f
}

// addForward inserts a new empty forward immediately after the focused forward
// (or after the last forward when a connection field is focused), and moves
// focus to the new forward's Name sub-field.
func (f tunnelForm) addForward() tunnelForm {
	at := len(f.forwards)
	if !f.onConnField() {
		at = f.focusedForward() + 1
	}
	forwards := make([]forwardFields, 0, len(f.forwards)+1)
	forwards = append(forwards, f.forwards[:at]...)
	forwards = append(forwards, newForwardFields())
	forwards = append(forwards, f.forwards[at:]...)
	f.forwards = forwards
	f.errMsg = ""
	f.focus = connFieldCount + at*subFieldsPerForward
	f.refocus()
	return f
}

// removeForward drops the focused forward and clamps focus into the remaining
// fields. A tunnel needs at least one forward, so removing the last one is
// refused with an explanatory errMsg. Removing while a connection field is
// focused is a no-op.
func (f tunnelForm) removeForward() tunnelForm {
	if f.onConnField() {
		return f
	}
	if len(f.forwards) == 1 {
		f.errMsg = "a tunnel needs at least one forward"
		return f
	}
	at := f.focusedForward()
	forwards := make([]forwardFields, 0, len(f.forwards)-1)
	forwards = append(forwards, f.forwards[:at]...)
	forwards = append(forwards, f.forwards[at+1:]...)
	f.forwards = forwards
	f.errMsg = ""
	if f.focus >= f.fieldCount() {
		f.focus = f.fieldCount() - 1
	}
	f.refocus()
	return f
}

// cycleMode advances the focused forward's mode by delta (wrapping).
func (f tunnelForm) cycleMode(delta int) tunnelForm {
	idx := f.focusedForward()
	cur := 0
	for i, m := range formModes {
		if m == f.forwards[idx].mode {
			cur = i
			break
		}
	}
	next := formModes[(cur+delta+len(formModes))%len(formModes)]
	forwards := make([]forwardFields, len(f.forwards))
	copy(forwards, f.forwards)
	forwards[idx].mode = next
	f.forwards = forwards
	return f
}

// forwardToInput passes the key to the focused text input, if any. A Mode
// selector is not a textinput, so a key on it is ignored.
//
// The `*in = updated` writes through a pointer into the forwards slice's
// backing array, which is still shared with the previous tunnelForm (the
// receiver is a value copy, but the slice header points at the same array).
// This aliasing write is SAFE only because the sole caller, routeFormKey in
// dashboard.go, immediately replaces d.form with the returned form and Bubble
// Tea is single-threaded, so the previous form is discarded the instant this
// returns. Do not cache a tunnelForm and expect it to stay immutable across an
// update.
func (f tunnelForm) forwardToInput(k tea.KeyMsg) tunnelForm {
	in := f.focusedInput()
	if in == nil {
		return f
	}
	updated, _ := in.Update(k) // cursor blink is not needed inside the form
	*in = updated
	return f
}

// connValue returns the trimmed value of the connection input at idx.
func (f tunnelForm) connValue(idx int) string {
	return strings.TrimSpace(f.conn[idx].Value())
}

// toDesc builds a tunnel.Desc from the form, returning any parse error. The
// connection fields populate the Desc; the sub-editor populates Forwards.
func (f tunnelForm) toDesc() (tunnel.Desc, error) {
	port, err := parseOptionalInt(f.conn[connPort].Value(), "port")
	if err != nil {
		return tunnel.Desc{}, err
	}
	keepAlive, err := parseOptionalInt(f.conn[connKeepAlive].Value(), "keep-alive")
	if err != nil {
		return tunnel.Desc{}, err
	}
	return tunnel.Desc{
		Name:         f.connValue(connName),
		Host:         f.connValue(connHost),
		User:         f.connValue(connUser),
		IdentityFile: f.connValue(connIdentity),
		Port:         port,
		KeepAlive:    keepAlive,
		Group:        f.connValue(connGroup),
		Forwards:     f.toForwards(),
	}, nil
}

// toForwards builds the tunnel.Forward slice from the sub-editor entries.
func (f tunnelForm) toForwards() []tunnel.Forward {
	forwards := make([]tunnel.Forward, len(f.forwards))
	for i := range f.forwards {
		ff := &f.forwards[i]
		forwards[i] = tunnel.Forward{
			Name:          strings.TrimSpace(ff.name.Value()),
			LocalAddress:  tunnel.StringOrInt(strings.TrimSpace(ff.local.Value())),
			RemoteAddress: tunnel.StringOrInt(strings.TrimSpace(ff.remote.Value())),
			Mode:          ff.mode,
		}
	}
	return forwards
}

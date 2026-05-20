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

// modeIndex is the focus index of the Mode selector. Focus values 0..3 map to
// inputs 0..3, focus 4 is the Mode selector, focus 5..9 map to inputs 4..8.
const modeIndex = 4

// formFieldCount is the number of focusable fields (9 text inputs + Mode).
const formFieldCount = 10

// formInputCount is the number of textinput fields (every field but Mode).
const formInputCount = formFieldCount - 1

// formModes lists the modes the selector cycles through, in order.
var formModes = []tunnel.Mode{
	tunnel.Local, tunnel.Remote, tunnel.Socks, tunnel.RemoteSocks,
}

// formLabels are the field labels in tab order. The Mode label sits at modeIndex.
var formLabels = []string{
	"Name", "Host", "Local", "Remote",
	"Mode",
	"User", "Identity", "Port", "KeepAlive", "Group",
}

// tunnelForm collects the fields of one tunnel.Desc for adding or editing.
type tunnelForm struct {
	inputs  []textinput.Model // the 9 text fields; Mode is not a textinput
	mode    tunnel.Mode
	focus   int    // 0..9; modeIndex is the Mode selector, others map to inputs
	editing string // name of the tunnel being edited; "" when adding a new one
	errMsg  string
}

// newTunnelForm builds an empty form for a new tunnel.
func newTunnelForm() tunnelForm {
	f := tunnelForm{
		inputs: make([]textinput.Model, formInputCount),
		mode:   tunnel.Local,
	}
	for i := range f.inputs {
		f.inputs[i] = textinput.New()
	}
	f.inputs[0].Focus()
	return f
}

// formFromDesc builds a form populated from d for editing. A field whose value
// equals config.SocksLabel is shown empty: that label is display-only and must
// not be presented back to the user or re-saved.
func formFromDesc(d tunnel.Desc) tunnelForm {
	f := newTunnelForm()
	f.mode = d.Mode
	f.editing = d.Name
	values := []string{
		d.Name,
		d.Host,
		blankSocksLabel(d.LocalAddress.String()),
		blankSocksLabel(d.RemoteAddress.String()),
		d.User,
		d.IdentityFile,
		intToString(d.Port),
		intToString(d.KeepAlive),
		d.Group,
	}
	for i, v := range values {
		f.inputs[i].SetValue(v)
	}
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

// inputIndex maps a focus value to the index in inputs it controls. It must
// only be called when focus is not modeIndex.
func inputIndex(focus int) int {
	if focus > modeIndex {
		return focus - 1
	}
	return focus
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
	case keyLeft:
		if f.focus == modeIndex {
			return f.cycleMode(-1), formNone
		}
	case keyRight:
		if f.focus == modeIndex {
			return f.cycleMode(1), formNone
		}
	}
	return f.forwardToInput(k), formNone
}

// moveFocus shifts focus by delta (wrapping) and re-focuses the active input.
func (f tunnelForm) moveFocus(delta int) tunnelForm {
	if f.focus != modeIndex {
		f.inputs[inputIndex(f.focus)].Blur()
	}
	f.focus = (f.focus + delta + formFieldCount) % formFieldCount
	if f.focus != modeIndex {
		f.inputs[inputIndex(f.focus)].Focus()
	}
	return f
}

// cycleMode advances the selected mode by delta (wrapping).
func (f tunnelForm) cycleMode(delta int) tunnelForm {
	cur := 0
	for i, m := range formModes {
		if m == f.mode {
			cur = i
			break
		}
	}
	f.mode = formModes[(cur+delta+len(formModes))%len(formModes)]
	return f
}

// forwardToInput passes the key to the focused text input, if any.
func (f tunnelForm) forwardToInput(k tea.KeyMsg) tunnelForm {
	if f.focus == modeIndex {
		return f
	}
	idx := inputIndex(f.focus)
	in := f.inputs[idx]
	var cmd tea.Cmd
	in, cmd = in.Update(k)
	_ = cmd // text input cursor blink is not needed inside the form
	f.inputs[idx] = in
	return f
}

// fieldValue returns the trimmed value of the text input at the given index.
func (f tunnelForm) fieldValue(idx int) string {
	return strings.TrimSpace(f.inputs[idx].Value())
}

// toDesc builds a tunnel.Desc from the form, returning any parse error.
func (f tunnelForm) toDesc() (tunnel.Desc, error) {
	port, err := parseOptionalInt(f.inputs[6].Value(), "port")
	if err != nil {
		return tunnel.Desc{}, err
	}
	keepAlive, err := parseOptionalInt(f.inputs[7].Value(), "keep-alive")
	if err != nil {
		return tunnel.Desc{}, err
	}
	return tunnel.Desc{
		Name:          f.fieldValue(0),
		Host:          f.fieldValue(1),
		LocalAddress:  tunnel.StringOrInt(f.fieldValue(2)),
		RemoteAddress: tunnel.StringOrInt(f.fieldValue(3)),
		User:          f.fieldValue(4),
		IdentityFile:  f.fieldValue(5),
		Port:          port,
		KeepAlive:     keepAlive,
		Group:         f.fieldValue(8),
		Mode:          f.mode,
	}, nil
}

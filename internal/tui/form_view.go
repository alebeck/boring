package tui

import (
	"fmt"
	"strings"
)

// formHint is the key reference shown at the bottom of the form.
const formHint = "tab/shift+tab move · ←/→ mode · ctrl+f add forward · " +
	"ctrl+x remove forward · enter save · esc cancel"

// formIndent prefixes a forward's sub-field rows so the sub-editor reads as an
// indented group under its heading.
const formIndent = "  "

// formTitle returns the form's heading: "Add tunnel" for a new tunnel,
// "Edit tunnel: <name>" when editing.
func (f tunnelForm) title() string {
	if f.editing == "" {
		return "Add tunnel"
	}
	return "Edit tunnel: " + f.editing
}

// formRow renders one "label: value" line, highlighting it when focused.
func formRow(label, value string, focused bool) string {
	line := formLabelStyle.Render(label+":") + " " + value
	if focused {
		return cursorStyle.Render(line)
	}
	return line
}

// View renders the form: title, the connection fields, the forwards
// sub-editor, and an error line (if any). The key hint is shown by the
// dashboard footer.
func (f tunnelForm) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(f.title()))
	b.WriteString("\n\n")
	b.WriteString(f.connRows())
	b.WriteString("\n\n")
	b.WriteString(f.forwardsView())
	if f.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(errStyle.Render(f.errMsg))
	}
	return b.String()
}

// connRows renders the connection fields, one labelled row each.
func (f tunnelForm) connRows() string {
	rows := make([]string, connFieldCount)
	for i := 0; i < connFieldCount; i++ {
		rows[i] = formRow(connLabels[i], f.conn[i].View(), i == f.focus)
	}
	return strings.Join(rows, "\n")
}

// forwardsView renders every forward as a small labelled group.
func (f tunnelForm) forwardsView() string {
	groups := make([]string, len(f.forwards))
	for i := range f.forwards {
		groups[i] = f.forwardGroup(i)
	}
	return strings.Join(groups, "\n\n")
}

// forwardGroup renders one forward: an indented heading and its four
// sub-fields (Name, Local, Remote, Mode).
func (f tunnelForm) forwardGroup(idx int) string {
	var b strings.Builder
	b.WriteString(formLabelStyle.Render(fmt.Sprintf("Forward %d", idx+1)))
	for sub := 0; sub < subFieldsPerForward; sub++ {
		b.WriteString("\n")
		b.WriteString(formIndent)
		b.WriteString(f.subRow(idx, sub))
	}
	return b.String()
}

// subRow renders one forward sub-field row, highlighting it when focused.
func (f tunnelForm) subRow(idx, sub int) string {
	focus := connFieldCount + idx*subFieldsPerForward + sub
	return formRow(subLabels[sub], f.subValue(idx, sub), focus == f.focus)
}

// subValue returns the displayed value of a forward sub-field: the Mode
// selector for subMode, otherwise the textinput's view.
func (f tunnelForm) subValue(idx, sub int) string {
	if sub == subMode {
		return "< " + f.forwards[idx].mode.ConfigValue() + " >"
	}
	return f.forwards[idx].input(sub).View()
}

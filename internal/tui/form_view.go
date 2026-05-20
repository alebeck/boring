package tui

import "strings"

// formHint is the key reference shown at the bottom of the form.
const formHint = "tab/shift+tab move · ←/→ mode · enter save · esc cancel"

// formTitle returns the form's heading: "Add tunnel" for a new tunnel,
// "Edit tunnel: <name>" when editing.
func (f tunnelForm) title() string {
	if f.editing == "" {
		return "Add tunnel"
	}
	return "Edit tunnel: " + f.editing
}

// modeValue is the canonical mode string shown in the Mode selector row.
func (f tunnelForm) modeValue() string {
	return f.mode.ConfigValue()
}

// formRow renders one "label: value" line, highlighting it when focused.
func formRow(label, value string, focused bool) string {
	line := formLabelStyle.Render(label+":") + " " + value
	if focused {
		return cursorStyle.Render(line)
	}
	return line
}

// View renders the form: title, every field as a labelled row, and an error
// line (if any). The key hint is shown by the dashboard footer.
func (f tunnelForm) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(f.title()))
	b.WriteString("\n\n")
	b.WriteString(f.fieldRows())
	if f.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(errStyle.Render(f.errMsg))
	}
	return b.String()
}

// fieldRows renders every field row in tab order.
func (f tunnelForm) fieldRows() string {
	rows := make([]string, formFieldCount)
	for focus := 0; focus < formFieldCount; focus++ {
		rows[focus] = formRow(formLabels[focus], f.rowValue(focus), focus == f.focus)
	}
	return strings.Join(rows, "\n")
}

// rowValue returns the displayed value for a field at the given focus index.
func (f tunnelForm) rowValue(focus int) string {
	if focus == modeIndex {
		return "< " + f.modeValue() + " >"
	}
	return f.inputs[inputIndex(focus)].View()
}

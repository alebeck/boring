package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
)

// authModal collects answers for one auth request, one question at a time.
type authModal struct {
	req     authRequestMsg
	input   textinput.Model
	idx     int      // index of the question currently being answered
	answers []string // answers collected so far
}

// newAuthModal builds a modal ready to collect the first answer for req.
func newAuthModal(req authRequestMsg) authModal {
	m := authModal{req: req, answers: make([]string, 0, len(req.questions))}
	m.input = textinput.New()
	m.input.Focus()
	m.configureInput()
	return m
}

// configureInput resets the text input for the current question, masking it
// when the question is non-echo (a passphrase / 2FA secret).
func (m *authModal) configureInput() {
	m.input.SetValue("")
	if m.idx < len(m.req.echo) && !m.req.echo[m.idx] {
		m.input.EchoMode = textinput.EchoPassword
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
}

// currentQuestion returns the prompt text for the question being answered.
func (m authModal) currentQuestion() string {
	if m.idx < len(m.req.questions) {
		return m.req.questions[m.idx]
	}
	return ""
}

// modalTitle returns a short heading naming the prompt's source.
func (m authModal) modalTitle() string {
	if m.req.name == "" {
		return "Authentication"
	}
	return "Authentication: " + m.req.name
}

// View renders the modal as a bordered box.
func (m authModal) View() string {
	var b strings.Builder
	b.WriteString(modalTitleStyle.Render(m.modalTitle()))
	b.WriteString("\n\n")
	if instr := strings.TrimSpace(m.req.instruction); instr != "" {
		b.WriteString(instr)
		b.WriteString("\n\n")
	}
	if q := strings.TrimSpace(m.currentQuestion()); q != "" {
		b.WriteString(q)
		b.WriteString("\n")
	}
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("enter submit · esc cancel"))
	return modalStyle.Render(b.String())
}

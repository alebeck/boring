package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// typeRunes feeds each rune of s to the dashboard as a key message and returns
// the resulting dashboard.
func typeRunes(d dashboard, s string) dashboard {
	for _, r := range s {
		m, _ := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		d = m.(dashboard)
	}
	return d
}

func TestAuthRequestOpensModal(t *testing.T) {
	d := dashboardWithRows("a")
	req := authRequestMsg{
		name:      "totp",
		questions: []string{"Code: "},
		echo:      []bool{false},
		reply:     make(chan authReply, 1),
	}
	m, _ := d.Update(req)
	if m.(dashboard).authModal == nil {
		t.Fatal("an auth request should open a modal")
	}
}

func TestAuthRequestQueuesWhenModalActive(t *testing.T) {
	d := dashboardWithRows("a")
	first := authRequestMsg{
		questions: []string{"q1"},
		echo:      []bool{false},
		reply:     make(chan authReply, 1),
	}
	second := authRequestMsg{
		questions: []string{"q2"},
		echo:      []bool{false},
		reply:     make(chan authReply, 1),
	}

	m, _ := d.Update(first)
	m, _ = m.(dashboard).Update(second)
	nd := m.(dashboard)

	if nd.authModal == nil {
		t.Fatal("the first request should still be shown")
	}
	if nd.authModal.req.questions[0] != "q1" {
		t.Fatalf("modal shows %q, want the first request", nd.authModal.req.questions[0])
	}
	if len(nd.authQueue) != 1 {
		t.Fatalf("queue length = %d, want 1", len(nd.authQueue))
	}
	if nd.authQueue[0].questions[0] != "q2" {
		t.Fatal("the second request should be queued")
	}
}

func TestAuthModalSubmitSendsReply(t *testing.T) {
	d := dashboardWithRows("a")
	reply := make(chan authReply, 1)
	req := authRequestMsg{
		name:      "totp",
		questions: []string{"Code: "},
		echo:      []bool{false},
		reply:     reply,
	}

	m, _ := d.Update(req)
	d = m.(dashboard)
	d = typeRunes(d, "654321")
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)

	select {
	case r := <-reply:
		if r.err != nil {
			t.Fatalf("reply carried error: %v", r.err)
		}
		if len(r.answers) != 1 || r.answers[0] != "654321" {
			t.Fatalf("reply answers = %v, want [654321]", r.answers)
		}
	default:
		t.Fatal("submitting the modal should send a reply")
	}
	if nd.authModal != nil {
		t.Fatal("the modal should close after the single question is answered")
	}
}

func TestAuthModalSubmitOpensNextQueued(t *testing.T) {
	d := dashboardWithRows("a")
	firstReply := make(chan authReply, 1)
	first := authRequestMsg{
		questions: []string{"q1"},
		echo:      []bool{false},
		reply:     firstReply,
	}
	second := authRequestMsg{
		questions: []string{"q2"},
		echo:      []bool{false},
		reply:     make(chan authReply, 1),
	}

	m, _ := d.Update(first)
	m, _ = m.(dashboard).Update(second)
	d = m.(dashboard)
	d = typeRunes(d, "answer")
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)

	if r := <-firstReply; r.answers[0] != "answer" {
		t.Fatalf("first reply = %v, want [answer]", r.answers)
	}
	if nd.authModal == nil {
		t.Fatal("the queued request should open after the first finishes")
	}
	if nd.authModal.req.questions[0] != "q2" {
		t.Fatalf("modal shows %q, want the queued request", nd.authModal.req.questions[0])
	}
	if len(nd.authQueue) != 0 {
		t.Fatalf("queue length = %d, want 0", len(nd.authQueue))
	}
}

func TestAuthModalMultiQuestion(t *testing.T) {
	d := dashboardWithRows("a")
	reply := make(chan authReply, 1)
	req := authRequestMsg{
		questions: []string{"q1", "q2"},
		echo:      []bool{true, false},
		reply:     reply,
	}

	m, _ := d.Update(req)
	d = m.(dashboard)
	d = typeRunes(d, "one")
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	d = m.(dashboard)

	// First answer recorded, modal still open for the second question.
	if d.authModal == nil {
		t.Fatal("modal should stay open until every question is answered")
	}
	if d.authModal.idx != 1 {
		t.Fatalf("modal idx = %d, want 1", d.authModal.idx)
	}

	d = typeRunes(d, "two")
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nd := m.(dashboard)

	r := <-reply
	if len(r.answers) != 2 || r.answers[0] != "one" || r.answers[1] != "two" {
		t.Fatalf("reply answers = %v, want [one two]", r.answers)
	}
	if nd.authModal != nil {
		t.Fatal("modal should close after both questions are answered")
	}
}

func TestAuthModalCancelAborts(t *testing.T) {
	d := dashboardWithRows("a")
	reply := make(chan authReply, 1)
	req := authRequestMsg{
		questions: []string{"Code: "},
		echo:      []bool{false},
		reply:     reply,
	}

	m, _ := d.Update(req)
	d = m.(dashboard)
	m, _ = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nd := m.(dashboard)

	r := <-reply
	if !errors.Is(r.err, auth.ErrAborted) {
		t.Fatalf("reply error = %v, want auth.ErrAborted", r.err)
	}
	if r.answers != nil {
		t.Fatalf("aborted reply answers = %v, want nil", r.answers)
	}
	if nd.authModal != nil {
		t.Fatal("the modal should close after esc")
	}
}

func TestAuthModalKeyDoesNotReachDashboard(t *testing.T) {
	// While a modal is up, q must edit the input, not quit the dashboard.
	d := dashboardWithRows("a")
	req := authRequestMsg{
		questions: []string{"Code: "},
		echo:      []bool{false},
		reply:     make(chan authReply, 1),
	}
	m, _ := d.Update(req)
	d = m.(dashboard)
	m, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if isQuit(cmd) {
		t.Fatal("q must not quit while a modal is shown")
	}
	if m.(dashboard).authModal == nil {
		t.Fatal("the modal should still be open after typing into it")
	}
}

func TestAuthModalViewShowsQuestion(t *testing.T) {
	req := authRequestMsg{
		name:        "totp",
		instruction: "Two-factor required",
		questions:   []string{"Verification code: "},
		echo:        []bool{false},
		reply:       make(chan authReply, 1),
	}
	out := newAuthModal(req).View()
	if !strings.Contains(out, "Verification code:") {
		t.Fatalf("modal view should show the question, got:\n%s", out)
	}
	if !strings.Contains(out, "Two-factor required") {
		t.Fatalf("modal view should show the instruction, got:\n%s", out)
	}
	if !strings.Contains(out, "esc cancel") {
		t.Fatalf("modal view should show the key hint, got:\n%s", out)
	}
}

func TestAuthModalMasksNonEchoInput(t *testing.T) {
	echo := newAuthModal(authRequestMsg{
		questions: []string{"q"},
		echo:      []bool{true},
	})
	if echo.input.EchoMode != textinput.EchoNormal {
		t.Fatal("echo question should display input normally")
	}
	hidden := newAuthModal(authRequestMsg{
		questions: []string{"q"},
		echo:      []bool{false},
	})
	if hidden.input.EchoMode != textinput.EchoPassword {
		t.Fatal("non-echo question should mask input")
	}
}

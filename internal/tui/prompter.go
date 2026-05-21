package tui

import (
	"github.com/alebeck/boring/internal/auth"
	tea "github.com/charmbracelet/bubbletea"
)

// authReply carries the user's answer back to a blocked tuiPrompter.Prompt call.
type authReply struct {
	answers []string
	err     error
}

// authRequestMsg is sent into the Bubble Tea program when an auth prompt is
// needed. The UI goroutine opens a modal for it and sends the result on reply.
type authRequestMsg struct {
	name        string
	instruction string
	questions   []string
	echo        []bool
	// reply receives exactly one authReply from the UI goroutine (on modal
	// submit, cancel, or quit-drain). It is buffered (cap 1) so that send
	// never blocks the UI goroutine.
	reply chan authReply
}

// tuiPrompter is an auth.Prompter that relays prompts to the TUI. Prompt runs
// on a command goroutine: it pushes an authRequestMsg into the program and
// blocks until the UI goroutine replies.
type tuiPrompter struct {
	send func(tea.Msg) // set to the tea.Program's Send after the program exists
}

func (p *tuiPrompter) Prompt(name, instruction string,
	questions []string, echo []bool) ([]string, error) {
	reply := make(chan authReply, 1)
	p.send(authRequestMsg{
		name:        name,
		instruction: instruction,
		questions:   questions,
		echo:        echo,
		reply:       reply,
	})
	r := <-reply
	return r.answers, r.err
}

var _ auth.Prompter = (*tuiPrompter)(nil)

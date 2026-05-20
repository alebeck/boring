package daemon

import (
	"bufio"
	"fmt"
	"net"

	"github.com/alebeck/boring/internal/auth"
)

// ipcPrompter is an auth.Prompter that relays prompts over an open IPC
// connection to the connected client (CLI or TUI). Each Prompt call writes
// an AuthPrompt and blocks until the client returns an AuthReply.
type ipcPrompter struct {
	conn net.Conn
	br   *bufio.Reader // shared reader for this connection
}

func newIPCPrompter(conn net.Conn, br *bufio.Reader) *ipcPrompter {
	return &ipcPrompter{conn: conn, br: br}
}

func (p *ipcPrompter) Prompt(name, instruction string,
	questions []string, echo []bool) ([]string, error) {
	prompt := AuthPrompt{
		Name:        name,
		Instruction: instruction,
		Questions:   questions,
		Echo:        echo,
	}
	if err := WriteMsg(p.conn, MsgAuthPrompt, prompt); err != nil {
		return nil, fmt.Errorf("failed to send auth prompt: %w", err)
	}
	env, err := ReadEnvelope(p.br)
	if err != nil {
		return nil, fmt.Errorf("failed to read auth reply: %w", err)
	}
	if env.Type != MsgAuthReply {
		return nil, fmt.Errorf("expected auth reply, got %q", env.Type)
	}
	reply, err := DecodeAuthReply(env)
	if err != nil {
		return nil, err
	}
	if reply.Err != "" {
		return nil, auth.ErrAborted
	}
	return reply.Answers, nil
}

var _ auth.Prompter = (*ipcPrompter)(nil)

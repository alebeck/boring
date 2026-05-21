//
// Client-side driver for the multi-message Open exchange. Shared by the CLI
// and the TUI so both can open tunnels and answer relayed auth prompts.
//

package daemon

import (
	"bufio"
	"fmt"
	"net"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/ipc"
)

// answerPrompt decodes one AuthPrompt, asks the user via prompter, and writes
// the AuthReply back to the daemon.
func answerPrompt(conn net.Conn, env Envelope, prompter auth.Prompter) error {
	p, err := DecodeAuthPrompt(env)
	if err != nil {
		return err
	}
	ans, perr := prompter.Prompt(p.Name, p.Instruction, p.Questions, p.Echo)
	reply := AuthReply{Answers: ans}
	if perr != nil {
		reply.Err = perr.Error()
	}
	return WriteMsg(conn, MsgAuthReply, reply)
}

// RunOpenExchange drives the multi-message Open exchange on conn: it sends cmd,
// answers any AuthPrompt messages via prompter, and returns the final Resp.
func RunOpenExchange(conn net.Conn, cmd Cmd, prompter auth.Prompter) (*Resp, error) {
	if err := ipc.Write(cmd, conn); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	for {
		env, err := ReadEnvelope(br)
		if err != nil {
			return nil, err
		}
		switch env.Type {
		case MsgAuthPrompt:
			if err := answerPrompt(conn, env, prompter); err != nil {
				return nil, err
			}
		case MsgResp:
			resp, err := DecodeResp(env)
			if err != nil {
				return nil, err
			}
			return &resp, nil
		default:
			return nil, fmt.Errorf("unexpected message type %q from daemon", env.Type)
		}
	}
}

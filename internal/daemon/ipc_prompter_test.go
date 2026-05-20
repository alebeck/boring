package daemon

import (
	"bufio"
	"errors"
	"net"
	"testing"

	"github.com/alebeck/boring/internal/auth"
)

func TestIPCPrompterRoundTrip(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	// Client side: read the AuthPrompt, reply with answers.
	go func() {
		br := bufio.NewReader(b)
		env, err := ReadEnvelope(br)
		if err != nil || env.Type != MsgAuthPrompt {
			return
		}
		_ = WriteMsg(b, MsgAuthReply, AuthReply{Answers: []string{"123456"}})
	}()

	p := newIPCPrompter(a, bufio.NewReader(a))
	ans, err := p.Prompt("2fa", "Enter code", []string{"Code:"}, []bool{false})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if len(ans) != 1 || ans[0] != "123456" {
		t.Fatalf("got %v, want [123456]", ans)
	}
}

func TestIPCPrompterAbort(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	// Client side: reply with an abort (non-empty Err).
	go func() {
		br := bufio.NewReader(b)
		if _, err := ReadEnvelope(br); err != nil {
			return
		}
		_ = WriteMsg(b, MsgAuthReply, AuthReply{Err: "user cancelled"})
	}()

	p := newIPCPrompter(a, bufio.NewReader(a))
	if _, err := p.Prompt("2fa", "", []string{"Code:"}, []bool{false}); !errors.Is(err, auth.ErrAborted) {
		t.Fatalf("got %v, want auth.ErrAborted", err)
	}
}

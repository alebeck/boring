package tui

import (
	"errors"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTuiPrompterPrompt(t *testing.T) {
	t.Run("returns the answers the UI replies with", func(t *testing.T) {
		p := &tuiPrompter{}
		p.send = func(msg tea.Msg) {
			req, ok := msg.(authRequestMsg)
			if !ok {
				t.Fatalf("expected authRequestMsg, got %T", msg)
			}
			req.reply <- authReply{answers: []string{"123456"}}
		}

		ans, err := p.Prompt("totp", "", []string{"Code: "}, []bool{false})
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
		if len(ans) != 1 || ans[0] != "123456" {
			t.Fatalf("Prompt returned %v, want [123456]", ans)
		}
	})

	t.Run("propagates an abort error from the UI", func(t *testing.T) {
		p := &tuiPrompter{}
		p.send = func(msg tea.Msg) {
			msg.(authRequestMsg).reply <- authReply{err: auth.ErrAborted}
		}

		ans, err := p.Prompt("totp", "", []string{"Code: "}, []bool{false})
		if !errors.Is(err, auth.ErrAborted) {
			t.Fatalf("Prompt error = %v, want auth.ErrAborted", err)
		}
		if ans != nil {
			t.Fatalf("Prompt answers = %v, want nil on error", ans)
		}
	})

	t.Run("forwards the request fields into the program", func(t *testing.T) {
		p := &tuiPrompter{}
		var got authRequestMsg
		p.send = func(msg tea.Msg) {
			got = msg.(authRequestMsg)
			got.reply <- authReply{answers: []string{"x"}}
		}

		_, err := p.Prompt("name", "instr",
			[]string{"q1", "q2"}, []bool{true, false})
		if err != nil {
			t.Fatalf("Prompt returned error: %v", err)
		}
		if got.name != "name" || got.instruction != "instr" {
			t.Fatalf("request carried name=%q instruction=%q", got.name, got.instruction)
		}
		if len(got.questions) != 2 || len(got.echo) != 2 {
			t.Fatalf("request questions/echo not forwarded: %+v", got)
		}
	})
}

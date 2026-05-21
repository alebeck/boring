package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestTerminalPrompterEcho(t *testing.T) {
	in := strings.NewReader("hello\n")
	var out bytes.Buffer
	p := &TerminalPrompter{in: in, out: &out}
	ans, err := p.Prompt("n", "instr", []string{"Name: "}, []bool{true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ans) != 1 || ans[0] != "hello" {
		t.Fatalf("got %q", ans)
	}
	if !strings.Contains(out.String(), "Name: ") {
		t.Fatalf("prompt not written: %q", out.String())
	}
}

func TestTerminalPrompterHiddenNoTTY(t *testing.T) {
	p := &TerminalPrompter{in: strings.NewReader("x\n"), out: &bytes.Buffer{}}
	_, err := p.Prompt("n", "", []string{"Code: "}, []bool{false})
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("want no-TTY error, got %v", err)
	}
}

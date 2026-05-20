package tunnel

import (
	"testing"

	"github.com/alebeck/boring/internal/auth"
)

func TestShouldReconnect(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		stopped     bool
		want        bool
	}{
		{"non-interactive unexpected drop reconnects", false, false, true},
		{"non-interactive stopped does not reconnect", false, true, false},
		{"interactive unexpected drop does not reconnect", true, false, false},
		{"interactive stopped does not reconnect", true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn := &Tunnel{Desc: &Desc{Name: "x"}, interactive: c.interactive}
			if got := tn.shouldReconnect(c.stopped); got != c.want {
				t.Fatalf("shouldReconnect(stopped=%v) = %v, want %v", c.stopped, got, c.want)
			}
		})
	}
}

func TestFinalStatus(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		stopped     bool
		want        Status
	}{
		{"non-interactive drop ends Closed", false, false, Closed},
		{"interactive drop ends NeedsAuth", true, false, NeedsAuth},
		{"interactive stopped ends Closed", true, true, Closed},
		{"non-interactive stopped ends Closed", false, true, Closed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn := &Tunnel{Desc: &Desc{Name: "x"}, interactive: c.interactive}
			if got := tn.finalStatus(c.stopped); got != c.want {
				t.Fatalf("finalStatus(stopped=%v) = %v, want %v", c.stopped, got, c.want)
			}
		})
	}
}

func TestInteractivePrompterTracksKeyboardInteractive(t *testing.T) {
	tn := &Tunnel{Desc: &Desc{Name: "x"}}
	inner := auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
		return []string{"answer"}, nil
	})
	p := &interactivePrompter{inner: inner, tunnel: tn}

	// A passphrase prompt must NOT mark the tunnel interactive.
	if _, err := p.Prompt(auth.PassphrasePromptName, "", []string{"Passphrase:"}, []bool{false}); err != nil {
		t.Fatal(err)
	}
	if tn.interactive {
		t.Fatal("passphrase prompt wrongly marked tunnel interactive")
	}

	// A keyboard-interactive (2FA) challenge must mark it interactive.
	if _, err := p.Prompt("", "2FA", []string{"Code:"}, []bool{false}); err != nil {
		t.Fatal(err)
	}
	if !tn.interactive {
		t.Fatal("keyboard-interactive challenge did not mark tunnel interactive")
	}
}

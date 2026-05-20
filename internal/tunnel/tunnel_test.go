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

// TestOpenSkipsPrepareWhenPrepared locks the mechanism that lets decrypted
// SSH signers survive a reconnect: once a tunnel is prepared, Open() must not
// re-run prepare() (which loads and decrypts keys, prompting the user). Every
// reconnect goes through Open() with prepared already true, so a passphrase is
// never requested twice. The tunnel points at an encrypted identity file, so
// if the prepared guard were removed, prepare() would decrypt it and bump the
// prompter count — which is exactly what this test forbids.
func TestOpenSkipsPrepareWhenPrepared(t *testing.T) {
	var count int
	counting := auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
		count++
		return []string{"testpass"}, nil
	})
	tn := FromDesc(&Desc{
		Name: "x", Host: "127.0.0.1", Mode: Local,
		LocalAddress: "9000", RemoteAddress: "localhost:9000",
		IdentityFile: "../../test/testdata/keys/client_enc",
	}, counting)
	tn.prepared = true // a tunnel that already connected once

	// With the prepared guard, Open() skips prepare() entirely: makeClient()
	// fails fast (no hops) and key loading never runs, so the prompter is
	// untouched. If the guard were removed, prepare() would run, decrypt the
	// passphrase-protected key, and bump the count.
	if err := tn.Open(); err == nil {
		t.Fatal("expected Open to fail: tunnel has no hops")
	}
	if count != 0 {
		t.Fatalf("prepare() re-ran despite prepared=true: prompter called %d times, want 0", count)
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

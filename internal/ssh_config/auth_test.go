package ssh_config

import (
	"testing"

	"github.com/alebeck/boring/internal/auth"
)

func TestBuildAuthMethodsIncludesKeyboardInteractive(t *testing.T) {
	withoutPrompter := buildAuthMethods(nil, nil)
	if len(withoutPrompter) != 1 {
		t.Fatalf("nil prompter: got %d methods, want 1 (public keys only)", len(withoutPrompter))
	}
	withPrompter := buildAuthMethods(nil, auth.FuncPrompter(
		func(_, _ string, _ []string, _ []bool) ([]string, error) { return nil, nil }))
	if len(withPrompter) != 2 {
		t.Fatalf("with prompter: got %d methods, want 2 (public keys + keyboard-interactive)", len(withPrompter))
	}
}

func TestKeyboardInteractiveChallenge(t *testing.T) {
	called := false
	cb := keyboardInteractiveChallenge(auth.FuncPrompter(
		func(_, _ string, qs []string, _ []bool) ([]string, error) {
			called = true
			return []string{"123456"}, nil
		}))

	// Informational challenge (zero questions): prompter must NOT be called.
	ans, err := cb("", "banner", nil, nil)
	if err != nil || ans != nil || called {
		t.Fatalf("informational challenge mishandled: ans=%v err=%v called=%v", ans, err, called)
	}

	// Real challenge: prompter delegated to, answers returned.
	ans, err = cb("name", "instr", []string{"Code: "}, []bool{false})
	if err != nil || len(ans) != 1 || ans[0] != "123456" || !called {
		t.Fatalf("delegation failed: ans=%v err=%v called=%v", ans, err, called)
	}
}

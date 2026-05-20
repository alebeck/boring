package auth

import (
	"errors"
	"testing"
)

func TestFuncPrompter(t *testing.T) {
	var gotQ []string
	p := FuncPrompter(func(name, instr string, qs []string, echo []bool) ([]string, error) {
		gotQ = qs
		return []string{"answer"}, nil
	})
	ans, err := p.Prompt("n", "i", []string{"Q?"}, []bool{false})
	if err != nil {
		t.Fatal(err)
	}
	if len(ans) != 1 || ans[0] != "answer" || gotQ[0] != "Q?" {
		t.Fatalf("got %v", ans)
	}
}

func TestPassphrase(t *testing.T) {
	t.Run("happy path returns the answer", func(t *testing.T) {
		p := FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return []string{"secret"}, nil
		})
		got, err := Passphrase(p, "/key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "secret" {
			t.Fatalf("got %q, want %q", got, "secret")
		}
	})

	t.Run("propagates prompter error", func(t *testing.T) {
		sentinel := errors.New("boom")
		p := FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return nil, sentinel
		})
		_, err := Passphrase(p, "/key")
		if !errors.Is(err, sentinel) {
			t.Fatalf("error %v does not wrap sentinel", err)
		}
	})

	t.Run("wrong answer count yields ErrAborted", func(t *testing.T) {
		p := FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return []string{"a", "b"}, nil
		})
		_, err := Passphrase(p, "/key")
		if !errors.Is(err, ErrAborted) {
			t.Fatalf("got %v, want ErrAborted", err)
		}
	})
}

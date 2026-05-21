package main

import (
	"io"
	"os"
	"testing"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/log"
)

func TestMain(m *testing.M) {
	log.Init(io.Discard, false, false)
	os.Exit(m.Run())
}

func TestSyncPrompterDelegates(t *testing.T) {
	p := &syncPrompter{inner: auth.FuncPrompter(
		func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return []string{"ok"}, nil
		})}
	ans, err := p.Prompt("n", "i", []string{"q"}, []bool{true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ans) != 1 || ans[0] != "ok" {
		t.Fatalf("got %v, want [ok]", ans)
	}
}

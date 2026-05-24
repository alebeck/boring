package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"~":     home,
		"~/foo": filepath.Join(home, "foo"),
		"/abs":  "/abs",
	}
	for in, want := range cases {
		if got := ReplaceTilde(in); got != want {
			t.Errorf("ReplaceTilde(%q) = %q, want %q", in, got, want)
		}
	}
}

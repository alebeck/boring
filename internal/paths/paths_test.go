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

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_VAR", "a")
	t.Setenv("TEST_EMPTY", "")

	cases := map[string]string{
		"/abs":                   "/abs",
		"${TEST_VAR}/x":          "a/x",
		"${TEST_VAR}${TEST_VAR}": "aa",
		"/x/${TEST_EMPTY}y":      "/x/y",
		"$TEST_VAR/x":            "$TEST_VAR/x",
	}
	for in, want := range cases {
		if got, err := ExpandEnv(in); err != nil || got != want {
			t.Errorf("ExpandEnv(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"${TEST_UNSET}", "${TEST_VAR"} {
		if _, err := ExpandEnv(in); err == nil {
			t.Errorf("ExpandEnv(%q): expected error", in)
		}
	}
}

package e2e

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env, "version")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %v", c, err)
	}
	if !strings.HasPrefix(out, "boring ") {
		t.Errorf("output did not start with 'boring ': %s", out)
	}
}

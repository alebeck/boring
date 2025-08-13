package e2e

import (
	"strings"
	"testing"
)

func TestUsage(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env)
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code should be 1, got %d", c)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("output did not contain usage information: %s", out)
	}
}

func TestUsageWithCommand(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env, "help") // or any other unknown command
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code should be 1, got %d", c)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("output did not contain usage information: %s", out)
	}
	if !strings.Contains(out, "Unknown command: help") {
		t.Errorf("output did not contain unknown command warning: %s", out)
	}
}

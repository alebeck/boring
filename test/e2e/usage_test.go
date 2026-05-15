package e2e

import (
	"strings"
	"testing"
)

func TestUsagePlain(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
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

func TestUsageHelp(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "help")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %v", c, err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("output did not contain usage information: %s", out)
	}
}

func TestUsageUnknownCommand(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "foo") // or any other unknown command
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code should be 1, got %d", c)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("output did not contain usage information: %s", out)
	}
	if !strings.Contains(out, "Unknown command: foo") {
		t.Errorf("output did not contain unknown command warning: %s", out)
	}
}


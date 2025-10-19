package e2e

import (
	"testing"

	"github.com/alebeck/boring/completions"
)

func TestBash(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "--shell", "bash")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if out != completions.Bash {
		t.Errorf("output completion was not correct: %s != %s", out, completions.Bash)
	}
}

func TestZsh(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "--shell", "zsh")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if out != completions.Zsh {
		t.Errorf("output completion was not correct: %s != %s", out, completions.Bash)
	}
}

func TestFish(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}

	c, out, err := cliCommand(env, "--shell", "fish")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if out != completions.Fish {
		t.Errorf("output completion was not correct: %s != %s", out, completions.Bash)
	}
}

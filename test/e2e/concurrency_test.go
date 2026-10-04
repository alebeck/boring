package e2e

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/sync/errgroup"
)

// runConcurrently runs a CLI command n times in parallel and fails the test
// on a non-zero exit code, unless the output contains an allowed message.
func runConcurrently(t *testing.T, env []string, n int, allowed []string, args ...string) {
	var g errgroup.Group
	outs := make([]string, n)
	codes := make([]int, n)
	for i := range n {
		g.Go(func() error {
			var err error
			codes[i], outs[i], err = cliCommand(env, args...)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	for i := range n {
		isAllowed := func(m string) bool { return strings.Contains(outs[i], m) }
		if codes[i] != 0 && !slices.ContainsFunc(allowed, isAllowed) {
			t.Fatalf("%v: exit code %d: %s", args, codes[i], outs[i])
		}
	}
}

// mustRun runs a CLI command and fails the test if it does not succeed.
func mustRun(t *testing.T, env []string, args ...string) {
	runConcurrently(t, env, 1, nil, args...)
}

// Messages of clients that lost the race, depending on how far the winner
// got: the tunnel is gone from the list, gone from the daemon, or already
// shut down but not yet removed.
var notRunning = []string{
	"No running tunnels match",
	"tunnel not running",
	"trying to close a closed tunnel",
}

// Closing the same tunnel from several clients at once used to crash the
// daemon with "close of closed channel". The daemon is started with
// BORING_NO_SPAWN, so a crash makes the following commands fail.
func TestCloseConcurrent(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	for range 5 {
		mustRun(t, env, "open", "test")
		runConcurrently(t, env, 4, notRunning, "close", "test")
		if s := listStatus(t, env); s != "closed" {
			t.Fatalf("expected tunnel to be closed, got %q", s)
		}
	}
}

// Opening the same tunnel from several clients at once must open it
// exactly once. The others should report it as already running instead
// of failing to bind the local port.
func TestOpenConcurrent(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	for range 5 {
		runConcurrently(t, env, 4, nil, "open", "test")
		if s := listStatus(t, env); s == "closed" {
			t.Fatal("expected tunnel to be open")
		}
		testTunnel(t, "localhost:49711", "localhost:49712")
		mustRun(t, env, "close", "test")
	}
}

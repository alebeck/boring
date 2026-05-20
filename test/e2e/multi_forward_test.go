package e2e

import (
	"strings"
	"testing"
)

// TestTunnelMultiForward proves the multi-forward runtime end to end: a single
// tunnel ("test-multi") with two [[tunnels.forward]] blocks opens one SSH
// connection and serves both forwards. Data must flow independently through
// each local listener to its own remote target, and closing the tunnel must
// tear both forwards down.
func TestTunnelMultiForward(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-multi")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(strings.ToLower(out), "opened tunnel") {
		t.Errorf("output did not indicate success: %s", out)
	}

	// Both forwards carry traffic independently over the one connection.
	testTunnel(t, "localhost:49731", "localhost:49732")
	testTunnel(t, "localhost:49733", "localhost:49734")

	// Closing the tunnel tears both forwards down.
	c, out, err = cliCommand(env, "close", "test-multi")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	if _, err := dial("localhost:49731"); err == nil {
		t.Error("first forward listener still accepting after close")
	}
	if _, err := dial("localhost:49733"); err == nil {
		t.Error("second forward listener still accepting after close")
	}
}

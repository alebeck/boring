package e2e

import (
	"regexp"
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
	if !strings.Contains(strings.ToLower(out), "closed tunnel") {
		t.Errorf("close output did not indicate success: %s", out)
	}

	if _, err := dial("localhost:49731"); err == nil {
		t.Error("first forward listener still accepting after close")
	}
	if _, err := dial("localhost:49733"); err == nil {
		t.Error("second forward listener still accepting after close")
	}
}

// TestTunnelMultiForwardList proves the daemon List response surfaces a
// multi-forward tunnel: while "test-multi" is open, `boring list` returns it
// with an open status, and after close it is reported closed. This exercises
// the daemon->CLI List path for a tunnel whose Desc carries [[tunnels.forward]]
// blocks rather than the legacy singular shorthand.
func TestTunnelMultiForwardList(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	if c, out, err := cliCommand(env, "open", "test-multi"); err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	} else if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(stripANSI(out), "test-multi") {
		t.Errorf("multi-forward tunnel not in list output: %s", out)
	}
	// The open tunnel's row carries an uptime status (e.g. "00m01s").
	uptime := regexp.MustCompile(`\d{2}m\d{2}s`)
	if !uptime.MatchString(stripANSI(out)) {
		t.Errorf("multi-forward tunnel not shown open in list output: %s", out)
	}

	if c, out, err := cliCommand(env, "close", "test-multi"); err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	} else if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	c, out, err = cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	listing := stripANSI(out)
	if !strings.Contains(listing, "test-multi") {
		t.Errorf("multi-forward tunnel missing from list after close: %s", out)
	}
	if !strings.Contains(listing, "closed") {
		t.Errorf("multi-forward tunnel not shown closed after close: %s", out)
	}
}

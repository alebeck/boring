package e2e

import (
	"regexp"
	"strings"
	"testing"
	"time"

	xproxy "golang.org/x/net/proxy"
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
	// The multi-forward tunnel renders as a grouped tree: a connection-level
	// header row plus one indented ├/└ sub-row per named forward.
	assertGroupedTree(t, stripANSI(out))

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
	// The grouped tree is rendered the same whether the tunnel is open or
	// closed — only the connection-level status differs.
	assertGroupedTree(t, listing)
}

// assertGroupedTree checks that `boring list` rendered the "test-multi" tunnel
// as a grouped tree: a header line carrying the tunnel name, followed by one
// indented ├/└ sub-row per forward ("first" and "second").
func assertGroupedTree(t *testing.T, listing string) {
	t.Helper()
	lines := strings.Split(listing, "\n")

	header := -1
	for i, l := range lines {
		if strings.Contains(l, "test-multi") {
			header = i
			break
		}
	}
	if header == -1 {
		t.Fatalf("test-multi header row not found in list output:\n%s", listing)
	}
	// The header row names the tunnel but not its forwards.
	if strings.Contains(lines[header], "first") || strings.Contains(lines[header], "second") {
		t.Errorf("forward labels leaked onto the tunnel header row: %q", lines[header])
	}
	if header+2 >= len(lines) {
		t.Fatalf("expected two forward sub-rows after the header:\n%s", listing)
	}
	first, second := lines[header+1], lines[header+2]
	if !strings.Contains(first, "├") || !strings.Contains(first, "first") {
		t.Errorf("first forward sub-row not rendered as ├ branch: %q", first)
	}
	if !strings.Contains(second, "└") || !strings.Contains(second, "second") {
		t.Errorf("second forward sub-row not rendered as └ branch: %q", second)
	}
}

// multiForwardStatus runs `boring list` and returns the normalized status of
// the named multi-forward tunnel: "open" for any uptime string, otherwise the
// literal status word ("closed", "reconn"). It returns "" when the tunnel is
// not listed. listStatus keys off the "->" arrow, which a multi-forward
// tunnel's connection-level header row does not carry (the arrow lives on the
// indented forward sub-rows), so the header row is matched by name directly:
// the row is `<status...> <name> <via>` with the status being one word.
func multiForwardStatus(t *testing.T, env []string, name string) string {
	t.Helper()
	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run list command: %v", err)
	}
	if c != 0 {
		t.Fatalf("list exit code %d: %s", c, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(stripANSI(out)), "\n") {
		if strings.Contains(line, "->") {
			continue // a forward sub-row, not the connection header
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != name {
			continue
		}
		if uptimeRe.MatchString(fields[0]) {
			return "open"
		}
		return fields[0]
	}
	return ""
}

// waitForMultiForwardStatus polls `boring list` until the named multi-forward
// tunnel reports want, or fails the test once the timeout elapses. Reconnect
// timing is non-deterministic, so reconnect assertions must poll rather than
// sleep on a fixed duration.
func waitForMultiForwardStatus(t *testing.T, env []string, name, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if multiForwardStatus(t, env, name) == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("multi-forward tunnel %q did not reach status %q in time (last: %q)",
				name, want, multiForwardStatus(t, env, name))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// TestMultiForwardReconnect proves that a multi-forward tunnel survives an
// unexpected SSH-connection drop: after the connection is severed, the tunnel
// reconnects and re-establishes EVERY forward over the fresh client. Task 2.1's
// runtime re-runs Open() on reconnect (re-establishing all forwards), and this
// test verifies that end to end — data must flow through both forwards both
// before the drop and again after the reconnect.
//
// The test name is kept short on purpose: t.TempDir() embeds it in the daemon
// Unix socket path, which macOS limits to ~104 bytes.
func TestMultiForwardReconnect(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-multi-reconn")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	// Both forwards carry traffic over the initial connection.
	testTunnel(t, "localhost:49741", "localhost:49742")
	testTunnel(t, "localhost:49743", "localhost:49744")

	// Sever the SSH connection the way the single-forward reconnect test does:
	// pause the server's accept loop and drop every live connection.
	server.pause()
	server.closeAll()

	// The tunnel detects the drop and enters the reconnecting state.
	waitForMultiForwardStatus(t, env, "test-multi-reconn", "reconn")

	// Restore the server and wait for the tunnel to come back to open.
	server.resume()
	waitForMultiForwardStatus(t, env, "test-multi-reconn", "open")

	// Both forwards must carry traffic again over the new connection — the
	// reconnect re-established every forward, not just the first.
	testTunnel(t, "localhost:49741", "localhost:49742")
	testTunnel(t, "localhost:49743", "localhost:49744")
}

// TestMultiForwardMixedModes proves that a single tunnel can carry forwards of
// DIFFERENT modes — a local, a remote, and a socks forward — over one shared
// SSH connection. Each forward is exercised according to its own mode; closing
// the tunnel tears all three down.
//
// The test name is kept short on purpose: t.TempDir() embeds it in the daemon
// Unix socket path, which macOS limits to ~104 bytes.
func TestMultiForwardMixedModes(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-multi-mixed")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	// Give the remote forward's server-side listener time to come up.
	time.Sleep(100 * time.Millisecond)

	// Local forward: connect to the local listener, data exits at the remote.
	testTunnel(t, "localhost:49761", "localhost:49762")

	// Remote forward: the server listens on the remote address and forwards
	// back to the local address, so connect to the remote, listen on local.
	testTunnel(t, "localhost:49764", "localhost:49763")

	// Socks forward: dial an arbitrary target through the SOCKS5 listener.
	socksDialer, err := xproxy.SOCKS5("tcp", "localhost:49765", nil, xproxy.Direct)
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	l, err := makeListener("localhost:49766")
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer l.Close()
	conn, err := socksDialer.Dial("tcp", "localhost:49766")
	if err != nil {
		t.Fatalf("%v", err.Error())
	}
	defer conn.Close()
	if err := testConnected(l, conn); err != nil {
		t.Fatalf("%v", err.Error())
	}

	// Closing the tunnel tears all three forwards down.
	c, out, err = cliCommand(env, "close", "test-multi-mixed")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if _, err := dial("localhost:49761"); err == nil {
		t.Error("local forward listener still accepting after close")
	}
	if _, err := dial("localhost:49765"); err == nil {
		t.Error("socks forward listener still accepting after close")
	}
}

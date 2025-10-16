package e2e

import (
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	xproxy "golang.org/x/net/proxy"
	"golang.org/x/sync/errgroup"
)

var server *sshServer

func TestMain(m *testing.M) {
	var err error
	if server, err = startServer(); err != nil {
		log.Fatalf("failed to start SSH server: %v", err)
	}
	defer server.cleanup()

	os.Exit(m.Run())
}

func TestList(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	lines := strings.Split(strings.TrimSpace(stripANSI(out)), "\n")

	// Check that header is present
	if !(reflect.DeepEqual(strings.Fields(lines[0]),
		[]string{"Status", "Name", "Local", "Remote", "Via"})) {
		t.Errorf("list output did not start with expected header: %s", out)
	}

	// Check for test tunnel in output
	if !(reflect.DeepEqual(strings.Fields(lines[1]),
		[]string{"closed", "test", "49711", "->", "localhost:49712", "127.0.0.1"})) {
		t.Errorf("test tunnel not in list output: %s", out)
	}
}

func TestListNoTunnels(t *testing.T) {
	cfg := defaultConfig
	cfg.boringConfig = t.TempDir() + "/config.toml"
	f, err := os.Create(cfg.boringConfig)
	if err != nil {
		t.Fatalf("could not create config file: %v", err)
	}
	f.Close()

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(out, "No tunnels configured.") {
		t.Fatalf("did not get expected output: %v", out)
	}
}

func TestOpen(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	c, out, err = cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	lines := strings.Split(strings.TrimSpace(stripANSI(out)), "\n")

	// Check that test tunnel is now open
	re := regexp.MustCompile(`^\d{2}m\d{2}s$`)
	if !re.MatchString(strings.Fields(lines[1])[0]) {
		t.Errorf("test tunnel not open in list output: %s", out)
	}
}

func TestOpenAll(t *testing.T) {
	cfg := defaultConfig
	cfg.boringConfig = "../testdata/config/config_small.toml"
	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "--all")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0", c)
	}

	out = stripANSI(out)
	if !strings.Contains(out, "Opened tunnel 'test'") || !strings.Contains(out, "Opened tunnel 'test2'") {
		t.Fatalf("output did not indicate opening both tunnels: %s", out)
	}
}

func TestOpenAlreadyRunning(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, _, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0", c)
	}

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0", c)
	}

	if !strings.Contains(out, "is already running") {
		t.Fatalf("output did not indicate tunnel is already running: %s", out)
	}
}

// Tests that we only support valid forwarding specifications as in ssh -L/R
func TestOpenBadRemoteConfig(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-bad-fwd-config")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}

	if !strings.Contains(out, "bad remote forwarding specification") {
		t.Fatalf("output did not indicate bad remote forwarding specification: %s", out)
	}
}

func TestOpenNoPattern(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env, "open")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}

	if !strings.Contains(out, "requires at least one 'pattern' argument") {
		t.Fatalf("output did not indicate failure: %s", out)
	}
}

func TestClose(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open
	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	// Close
	c, out, err = cliCommand(env, "close", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	c, out, err = cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	lines := strings.Split(strings.TrimSpace(stripANSI(out)), "\n")

	// Check that test tunnel is closed
	if strings.Fields(lines[1])[0] != "closed" {
		t.Errorf("test tunnel not closed in list output: %s", out)
	}
}

func TestCloseNotRunning(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "close", "doesnotexist", "neitherthatone")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}
	if !strings.Contains(out, "No running tunnels match any provided pattern") {
		t.Errorf("output did not indicate no running tunnels: %s", out)
	}
}

func TestCloseWarnNotRunning(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	c, out, err = cliCommand(env, "close", "doesnotexist", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d, should be 0", c)
	}
	if !strings.Contains(out, "No running tunnels match pattern 'doesnotexist'") {
		t.Errorf("output did not contain warning: %s", out)
	}
}

func TestCloseNoPattern(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env, "close")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}

	if !strings.Contains(out, "requires at least one 'pattern' argument") {
		t.Fatalf("output did not indicate failure: %s", out)
	}
}

func TestCloseAll(t *testing.T) {
	// TODO
}

func TestAllWithArgument(t *testing.T) {
	env, err := makeDefaultEnv(t)
	if err != nil {
		t.Fatalf(err.Error())
	}

	c, out, err := cliCommand(env, "close", "--all", "arg")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}
	if !strings.Contains(out, "'--all' does not take any additional arguments") {
		t.Errorf("output did not contain correct error message: %s", out)
	}
}

func TestMalformedGlob(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "[")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 1 {
		t.Fatalf("exit code %d, should be 1", c)
	}

	if !strings.Contains(out, "Malformed glob pattern") {
		t.Fatalf("output did not indicate malformed glob pattern: %s", out)
	}
}

func makeListener(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}
	l.(*net.TCPListener).SetDeadline(time.Now().Add(connTimeout))
	return l, nil
}

func dial(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to forwarded port: %v", err)
	}
	return conn, nil
}

func testConnected(l net.Listener, conn net.Conn) error {
	_, err := conn.Write(testMsg)
	if err != nil {
		return fmt.Errorf("failed to write: %v", err)
	}

	// Check it comes out on the other end
	newConn, err := l.Accept()
	if err != nil {
		return fmt.Errorf("failed to accept connection: %v", err)
	}
	defer newConn.Close()

	buf := make([]byte, len(testMsg))
	_, err = newConn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read: %v", err)
	}
	if string(buf) != string(testMsg) {
		return fmt.Errorf("expected %q, got %q", testMsg, buf)
	}

	return nil
}

func testTunnel(t *testing.T, from, to string) {
	l, err := makeListener(to)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer l.Close()
	conn, err := dial(from)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer conn.Close()

	if err := testConnected(l, conn); err != nil {
		t.Fatalf(err.Error())
	}
}

// Test simple forward tunnel
func TestTunnelLocal(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	if !strings.Contains(strings.ToLower(out), "opened tunnel") {
		t.Errorf("output did not indicate success: %s", out)
	}

	testTunnel(t, "localhost:49711", "localhost:49712")
}

// Test handling of multiple simultaneous connections
func TestTunnelMultiConns(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	l, err := makeListener("localhost:49712")
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer l.Close()

	conns := make([]net.Conn, 0, 100)
	for range 100 {
		conn, err := dial("localhost:49711")
		if err != nil {
			t.Fatalf(err.Error())
		}
		defer conn.Close()
		conns = append(conns, conn)
	}

	var eg errgroup.Group
	for _, c := range conns {
		eg.Go(func() error {
			return testConnected(l, c)
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatalf("error in concurrent connections: %v", err)
	}
}

func TestTunnelMultiTunnels(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test", "test2")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	out = stripANSI(out)
	if !strings.Contains(strings.ToLower(out), "opened tunnel 'test'") ||
		!strings.Contains(strings.ToLower(out), "opened tunnel 'test2'") {
		t.Errorf("output did not indicate success: %s", out)
	}

	testTunnel(t, "localhost:49711", "localhost:49712")
	testTunnel(t, "localhost:49713", "localhost:49714")
}

// Test a reverse connection
func TestTunnelRemote(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-remote")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	// Give the server listener some time to start
	time.Sleep(100 * time.Millisecond)

	testTunnel(t, "localhost:49712", "localhost:49711")
}

func TestOpenManualConfig(t *testing.T) {
	cfg := defaultConfig
	// Only provides known hosts, everything else has to be configured manually
	cfg.sshConfig = "../testdata/config/ssh_config_kh_only"

	env, cancel, err := makeEnvWithDaemon(cfg, t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test-manual")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
}

func TestTunnelReconnect(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	time.Sleep(50 * time.Millisecond) // Give the tunnel some time to establish

	server.pause()
	server.closeAll()

	// verify tunnel is in Reconn state
	c, out, err = cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	lines := strings.Split(strings.TrimSpace(stripANSI(out)), "\n")

	if strings.Fields(lines[1])[0] != "reconn" {
		t.Errorf("test tunnel not reconnecting in list: %s", out)
	}

	// Reconnect the server
	server.resume()
	time.Sleep(500 * time.Millisecond) // Plenty of time for reconnection

	testTunnel(t, "localhost:49711", "localhost:49712")
}

func TestTunnelReconnectAbort(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	c, out, err := cliCommand(env, "open", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	time.Sleep(50 * time.Millisecond) // Give the tunnel some time to establish

	server.pause()
	server.closeAll()

	// stop during reconnect
	c, out, err = cliCommand(env, "close", "test")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	server.resume()

	c, out, err = cliCommand(env, "list")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}
	lines := strings.Split(strings.TrimSpace(stripANSI(out)), "\n")

	// Check that test tunnel is closed
	if strings.Fields(lines[1])[0] != "closed" {
		t.Errorf("test tunnel not closed in list: %s", out)
	}
}

func TestTunnelJump(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test-jump")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	testTunnel(t, "localhost:49711", "localhost:49712")
}

func TestTunnelSocks(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test-socks")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	// Test basic connection through SOCKS5 proxy, unit tests should do the rest
	socksDialer, err := xproxy.SOCKS5("tcp", "localhost:49717", nil, xproxy.Direct)
	if err != nil {
		t.Fatal(err)
	}

	l, err := makeListener("localhost:49718")
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer l.Close()

	conn, err := socksDialer.Dial("tcp", "localhost:49718")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := testConnected(l, conn); err != nil {
		t.Fatalf(err.Error())
	}
}

func TestTunnelSocksRemote(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test-socks-remote")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	// This is now the "remote" host
	socksDialer, err := xproxy.SOCKS5("tcp", "localhost:49717", nil, xproxy.Direct)
	if err != nil {
		t.Fatal(err)
	}

	l, err := makeListener("localhost:49718")
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer l.Close()

	conn, err := socksDialer.Dial("tcp", "localhost:49718")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := testConnected(l, conn); err != nil {
		t.Fatalf(err.Error())
	}
}

// Test simple forward tunnel
func TestTunnelKeepAlive(t *testing.T) {
	env, cancel, err := makeDefaultEnvWithDaemon(t)
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer cancel()

	// Open tunnel via Command
	c, out, err := cliCommand(env, "open", "test-keepalive")
	if err != nil {
		t.Fatalf("failed to run CLI command: %v", err)
	}
	if c != 0 {
		t.Fatalf("exit code %d: %s", c, out)
	}

	server.resetKeepAlives()

	// keep-alive should be sent within a second for this tunnel
	time.Sleep(1100 * time.Millisecond)

	if server.keepAlives != 1 {
		t.Fatalf("expected 1 keep-alive, got %d", server.keepAlives)
	}
}

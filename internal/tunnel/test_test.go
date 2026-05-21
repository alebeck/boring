package tunnel

import (
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/log"
)

// TestMain initializes the package-global logger so code paths exercised by
// the tunnel tests (e.g. ssh_config logging) do not dereference a nil logger.
func TestMain(m *testing.M) {
	log.Init(io.Discard, false, false)
	os.Exit(m.Run())
}

// ptr returns a pointer to i, for building *int fields in tests.
func ptr(i int) *int { return &i }

func TestConnectionUnreachable(t *testing.T) {
	d := &Desc{
		Name: "x", Host: "127.0.0.1", Port: ptr(1),
		Forwards: []Forward{
			{LocalAddress: "9000", RemoteAddress: "localhost:9000", Mode: Local},
		},
	}
	res := TestConnection(d, nil)
	if res.OK {
		t.Fatal("expected failure for unreachable host")
	}
	if res.Err == "" {
		t.Fatal("expected a non-empty error message")
	}
}

// TestConnectionMultiForwardOpensNoListener locks the central property of
// TestConnection for the multi-forward model: it verifies only the SSH
// connection and opens no listener, regardless of how many forwards the Desc
// carries. Both forwards' local ports are occupied before the call; if
// TestConnection bound a listener, it would fail with an address-in-use error.
// Instead it must fail purely on the unreachable handshake, proving it never
// touched a forward's listener.
func TestConnectionMultiForwardOpensNoListener(t *testing.T) {
	// Occupy two local ports so any attempt to bind a forward listener fails.
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not occupy a port: %v", err)
	}
	defer first.Close()
	second, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not occupy a port: %v", err)
	}
	defer second.Close()

	d := &Desc{
		Name: "multi", Host: "127.0.0.1", Port: ptr(1),
		Forwards: []Forward{
			{Name: "a", LocalAddress: StringOrInt(first.Addr().String()),
				RemoteAddress: "localhost:9000", Mode: Local},
			{Name: "b", LocalAddress: StringOrInt(second.Addr().String()),
				RemoteAddress: "localhost:9001", Mode: Local},
		},
	}

	res := TestConnection(d, nil)
	if res.OK {
		t.Fatal("expected failure for unreachable host")
	}
	if res.Err == "" {
		t.Fatal("expected a non-empty error message")
	}
	// The failure must come from the SSH handshake, not from binding a
	// forward's listener on an already-occupied port.
	for _, bindErr := range []string{"address already in use", "cannot listen", "bind"} {
		if strings.Contains(res.Err, bindErr) {
			t.Fatalf("TestConnection appears to have bound a listener: %q", res.Err)
		}
	}
}

package tunnel

import (
	"io"
	"os"
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
		Name: "x", Host: "127.0.0.1", Port: ptr(1), Mode: Local,
		LocalAddress: "9000", RemoteAddress: "localhost:9000",
	}
	res := TestConnection(d, nil)
	if res.OK {
		t.Fatal("expected failure for unreachable host")
	}
	if res.Err == "" {
		t.Fatal("expected a non-empty error message")
	}
}

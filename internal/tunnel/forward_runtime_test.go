package tunnel

import (
	"net"
	"strings"
	"testing"
)

// TestParseForwardAddresses checks that parseForward parses a forward's local
// and remote addresses according to the forward's own mode.
func TestParseForwardAddresses(t *testing.T) {
	cases := []struct {
		name       string
		forward    Forward
		wantLocal  address
		wantRemote address
	}{
		{
			name:       "local mode full addresses",
			forward:    Forward{LocalAddress: "9000", RemoteAddress: "localhost:9000", Mode: Local},
			wantLocal:  address{"localhost:9000", "tcp"},
			wantRemote: address{"localhost:9000", "tcp"},
		},
		{
			name:       "remote mode allows bare remote port",
			forward:    Forward{LocalAddress: "localhost:9000", RemoteAddress: "9000", Mode: Remote},
			wantLocal:  address{"localhost:9000", "tcp"},
			wantRemote: address{"localhost:9000", "tcp"},
		},
		{
			name:       "unix socket local address",
			forward:    Forward{LocalAddress: "/tmp/sock", RemoteAddress: "localhost:9000", Mode: Local},
			wantLocal:  address{"/tmp/sock", "unix"},
			wantRemote: address{"localhost:9000", "tcp"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr, err := parseForward(c.forward)
			if err != nil {
				t.Fatalf("parseForward: %v", err)
			}
			if *fr.localAddr != c.wantLocal {
				t.Errorf("localAddr = %+v, want %+v", *fr.localAddr, c.wantLocal)
			}
			if *fr.remoteAddr != c.wantRemote {
				t.Errorf("remoteAddr = %+v, want %+v", *fr.remoteAddr, c.wantRemote)
			}
		})
	}
}

// TestParseForwardBadRemote checks that a bare port on the local side of a
// local-mode forward is rejected, exactly as for a standalone tunnel.
func TestParseForwardBadRemote(t *testing.T) {
	_, err := parseForward(Forward{LocalAddress: "9000", RemoteAddress: "9000", Mode: Local})
	if err == nil {
		t.Fatal("expected error for bare-port remote in local mode")
	}
	if !strings.Contains(err.Error(), "bad remote forwarding specification") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPrepareForwardsBuildsOneRuntimePerForward checks that prepareForwards
// builds one forwardRuntime per Forward, in order, with parsed addresses.
func TestPrepareForwardsBuildsOneRuntimePerForward(t *testing.T) {
	tn := &Tunnel{Desc: &Desc{
		Name: "multi",
		Forwards: []Forward{
			{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432", Mode: Local},
			{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379", Mode: Local},
		},
	}}

	if err := tn.prepareForwards(); err != nil {
		t.Fatalf("prepareForwards: %v", err)
	}
	if len(tn.forwards) != 2 {
		t.Fatalf("expected 2 forward runtimes, got %d", len(tn.forwards))
	}
	if tn.forwards[0].label() != "db" || tn.forwards[1].label() != "cache" {
		t.Errorf("forward order/labels wrong: %q, %q",
			tn.forwards[0].label(), tn.forwards[1].label())
	}
	if tn.forwards[0].localAddr.addr != "localhost:5432" {
		t.Errorf("db localAddr = %q, want localhost:5432", tn.forwards[0].localAddr.addr)
	}
	if tn.forwards[1].remoteAddr.addr != "redis:6379" {
		t.Errorf("cache remoteAddr = %q, want redis:6379", tn.forwards[1].remoteAddr.addr)
	}
}

// TestPrepareForwardsRejectsEmpty checks that a tunnel with no forwards is
// rejected — every loaded tunnel must carry at least one forward.
func TestPrepareForwardsRejectsEmpty(t *testing.T) {
	tn := &Tunnel{Desc: &Desc{Name: "empty"}}
	if err := tn.prepareForwards(); err == nil {
		t.Fatal("expected error for a tunnel with no forwards")
	}
}

// TestPrepareForwardsNamesOffendingForward checks that a parse failure names
// the offending forward by its label.
func TestPrepareForwardsNamesOffendingForward(t *testing.T) {
	tn := &Tunnel{Desc: &Desc{
		Name: "bad",
		Forwards: []Forward{
			{Name: "ok", LocalAddress: "5432", RemoteAddress: "db:5432", Mode: Local},
			{Name: "broken", LocalAddress: "9000", RemoteAddress: "9000", Mode: Local},
		},
	}}
	err := tn.prepareForwards()
	if err == nil {
		t.Fatal("expected error for the broken forward")
	}
	if !strings.Contains(err.Error(), `"broken"`) {
		t.Fatalf("error should name the offending forward, got: %v", err)
	}
}

// TestMakeListenersAtomicRollback checks that makeListeners is atomic: if a
// later forward's listener cannot bind, every listener already created in the
// call is closed, and the returned error names the offending forward.
func TestMakeListenersAtomicRollback(t *testing.T) {
	// Occupy a port so the second forward's bind fails.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not occupy a port: %v", err)
	}
	defer occupied.Close()
	takenAddr := occupied.Addr().String()

	tn := &Tunnel{Desc: &Desc{
		Name: "atomic",
		Forwards: []Forward{
			{Name: "first", LocalAddress: "127.0.0.1:0", RemoteAddress: "x:1", Mode: Local},
			{Name: "second", LocalAddress: StringOrInt(takenAddr), RemoteAddress: "x:1", Mode: Local},
		},
	}}
	if err := tn.prepareForwards(); err != nil {
		t.Fatalf("prepareForwards: %v", err)
	}

	err = tn.makeListeners()
	if err == nil {
		t.Fatal("expected makeListeners to fail on the taken port")
	}
	if !strings.Contains(err.Error(), `"second"`) {
		t.Fatalf("error should name the offending forward, got: %v", err)
	}

	// The first forward's listener must have been closed and cleared.
	if tn.forwards[0].listener != nil {
		t.Error("first forward's listener was not closed on rollback")
	}
	if tn.forwards[1].listener != nil {
		t.Error("second forward's listener should be nil after a failed bind")
	}
}

// TestMakeListenersSuccess checks the happy path: every forward gets a bound
// listener, and closeListeners tears them all down.
func TestMakeListenersSuccess(t *testing.T) {
	tn := &Tunnel{Desc: &Desc{
		Name: "ok",
		Forwards: []Forward{
			{Name: "a", LocalAddress: "127.0.0.1:0", RemoteAddress: "x:1", Mode: Local},
			{Name: "b", LocalAddress: "127.0.0.1:0", RemoteAddress: "x:1", Mode: Local},
		},
	}}
	if err := tn.prepareForwards(); err != nil {
		t.Fatalf("prepareForwards: %v", err)
	}
	if err := tn.makeListeners(); err != nil {
		t.Fatalf("makeListeners: %v", err)
	}
	for i, fr := range tn.forwards {
		if fr.listener == nil {
			t.Errorf("forward %d has no listener", i)
		}
	}
	tn.closeListeners()
	for i, fr := range tn.forwards {
		if fr.listener != nil {
			t.Errorf("forward %d listener not cleared after closeListeners", i)
		}
	}
}

// TestForwardLabelFallsBackToLocalAddress checks that an unnamed forward is
// labelled by its local address, while a named forward uses its Name.
func TestForwardLabelFallsBackToLocalAddress(t *testing.T) {
	if got := (Forward{LocalAddress: "9000"}).label(); got != "9000" {
		t.Errorf("unnamed label() = %q, want %q", got, "9000")
	}
	if got := (Forward{Name: "db", LocalAddress: "9000"}).label(); got != "db" {
		t.Errorf("named label() = %q, want %q", got, "db")
	}
}

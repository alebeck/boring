package tunnel

import (
	"reflect"
	"sync"
	"testing"
)

// openTunnel returns a tunnel in the state Open leaves it in, without
// connecting anywhere.
func openTunnel() *Tunnel {
	return &Tunnel{
		Desc:   &Desc{Name: "test", Status: Open},
		stop:   make(chan struct{}),
		Closed: make(chan struct{}),
	}
}

func TestCloseTwice(t *testing.T) {
	tun := openTunnel()
	if err := tun.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// The tunnel stays open until its run loop notices the stop signal,
	// so a second close in that window must not panic.
	if err := tun.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	select {
	case <-tun.stop:
	default:
		t.Fatal("stop channel not closed")
	}
}

func TestCloseConcurrent(t *testing.T) {
	tun := openTunnel()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if err := tun.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
		})
	}
	wg.Wait()
}

func TestCloseClosed(t *testing.T) {
	tun := openTunnel()
	tun.Status = Closed
	if err := tun.Close(); err == nil {
		t.Fatal("expected error when closing a closed tunnel")
	}
}

// Run with -race: status updates from the tunnel's goroutines must not race
// with the daemon taking snapshots for a listing.
func TestSnapshotConcurrent(t *testing.T) {
	tun := openTunnel()
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			tun.setStatus(Reconn)
			tun.setStatus(Open)
		}
	})
	for range 1000 {
		if s := tun.Snapshot().Status; s != Open && s != Reconn {
			t.Fatalf("unexpected status %v", s)
		}
	}
	wg.Wait()
}

func TestParseAddrs(t *testing.T) {
	tests := []struct {
		addr       string
		allowShort bool
		want       []address // nil means an error is expected
	}{
		{"9000", true, []address{{"localhost:9000", "tcp"}}},
		{"localhost:9000", false, []address{{"localhost:9000", "tcp"}}},
		{"[::1]:9000", false, []address{{"[::1]:9000", "tcp"}}},
		{"/tmp/x.sock", false, []address{{"/tmp/x.sock", "unix"}}},
		{"8000-8002,443", true, []address{
			{"localhost:8000", "tcp"}, {"localhost:8001", "tcp"},
			{"localhost:8002", "tcp"}, {"localhost:443", "tcp"},
		}},
		{"db:5432,6000-6001", false, []address{
			{"db:5432", "tcp"}, {"db:6000", "tcp"}, {"db:6001", "tcp"},
		}},
		{"9000", false, nil},
		{"8000-8001", false, nil},
		{"localhost:0", false, nil},
		{"localhost:65536", false, nil},
		{"localhost:9002-9000", false, nil},
		{"localhost:80,", false, nil},
		{"localhost:http", false, nil},
	}
	for _, tt := range tests {
		got, err := parseAddrs(tt.addr, tt.allowShort)
		if tt.want == nil {
			if err == nil {
				t.Errorf("%q: expected error", tt.addr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.addr, err)
			continue
		}
		var vals []address
		for _, a := range got {
			vals = append(vals, *a)
		}
		if !reflect.DeepEqual(vals, tt.want) {
			t.Errorf("%q: got %v, want %v", tt.addr, vals, tt.want)
		}
	}
}

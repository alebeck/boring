package tunnel

import (
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

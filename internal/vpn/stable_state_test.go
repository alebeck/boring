package vpn

import (
	"testing"
	"time"
)

func TestStableState(t *testing.T) {
	start := time.Unix(100, 0)
	state := NewStableState(30 * time.Second)

	if _, _, ok := state.Observe(true, start); ok {
		t.Fatal("Observe() ok = true on first observation, want false")
	}

	if _, _, ok := state.Observe(true, start.Add(29*time.Second)); ok {
		t.Fatal("Observe() ok = true before stable window, want false")
	}

	stable, changed, ok := state.Observe(true, start.Add(30*time.Second))
	if !ok || !stable || !changed {
		t.Fatalf("Observe() = (%v, %v, %v), want (true, true, true)", stable, changed, ok)
	}

	stable, changed, ok = state.Observe(true, start.Add(31*time.Second))
	if !ok || !stable || changed {
		t.Fatalf("Observe() = (%v, %v, %v), want (true, false, true)", stable, changed, ok)
	}
}

func TestStableStateChangeResetsWindow(t *testing.T) {
	start := time.Unix(100, 0)
	state := NewStableState(30 * time.Second)

	state.Observe(true, start)
	state.Observe(true, start.Add(30*time.Second))

	if _, _, ok := state.Observe(false, start.Add(31*time.Second)); ok {
		t.Fatal("Observe() ok = true immediately after state change, want false")
	}

	stable, changed, ok := state.Observe(false, start.Add(61*time.Second))
	if !ok || stable || !changed {
		t.Fatalf("Observe() = (%v, %v, %v), want (false, true, true)", stable, changed, ok)
	}
}

func TestStableStateZeroDuration(t *testing.T) {
	state := NewStableState(0)
	stable, changed, ok := state.Observe(true, time.Unix(100, 0))
	if !ok || !stable || !changed {
		t.Fatalf("Observe() = (%v, %v, %v), want (true, true, true)", stable, changed, ok)
	}
}

package tunnel

import "testing"

func TestShouldReconnect(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		stopped     bool
		want        bool
	}{
		{"non-interactive unexpected drop reconnects", false, false, true},
		{"non-interactive stopped does not reconnect", false, true, false},
		{"interactive unexpected drop does not reconnect", true, false, false},
		{"interactive stopped does not reconnect", true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn := &Tunnel{Desc: &Desc{Name: "x"}, interactive: c.interactive}
			if got := tn.shouldReconnect(c.stopped); got != c.want {
				t.Fatalf("shouldReconnect(stopped=%v) = %v, want %v", c.stopped, got, c.want)
			}
		})
	}
}

func TestFinalStatus(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		stopped     bool
		want        Status
	}{
		{"non-interactive drop ends Closed", false, false, Closed},
		{"interactive drop ends NeedsAuth", true, false, NeedsAuth},
		{"interactive stopped ends Closed", true, true, Closed},
		{"non-interactive stopped ends Closed", false, true, Closed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tn := &Tunnel{Desc: &Desc{Name: "x"}, interactive: c.interactive}
			if got := tn.finalStatus(c.stopped); got != c.want {
				t.Fatalf("finalStatus(stopped=%v) = %v, want %v", c.stopped, got, c.want)
			}
		})
	}
}

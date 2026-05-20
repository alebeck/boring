package main

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

// ansiRe strips ANSI escape sequences so rendered output can be asserted as
// plain text.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// TestTunnelTableSingleForward proves a single-forward tunnel renders inline:
// one line carrying status, name, the forward's local/mode/remote, and the
// host — essentially the legacy `boring list` row.
func TestTunnelTableSingleForward(t *testing.T) {
	log.Init(io.Discard, false, false)
	d := &tunnel.Desc{
		Name:   "dev",
		Host:   "devhost",
		Status: tunnel.Closed,
		Forwards: []tunnel.Forward{
			{LocalAddress: "9000", RemoteAddress: "localhost:9000", Mode: tunnel.Local},
		},
	}
	out := stripANSI(tunnelTable([]*tunnel.Desc{d}).String())
	rows := nonEmptyLines(out)
	if len(rows) != 2 {
		t.Fatalf("single-forward tunnel must render header + 1 row, got %d lines: %q", len(rows), rows)
	}
	want := []string{"closed", "dev", "9000", "->", "localhost:9000", "devhost"}
	if got := strings.Fields(rows[1]); !equalSlice(got, want) {
		t.Errorf("inline row = %v, want %v", got, want)
	}
}

// TestTunnelTableMultiForward proves a multi-forward tunnel renders a
// connection-level header row plus one indented ├/└ sub-row per forward, each
// showing the forward's label and local -> remote.
func TestTunnelTableMultiForward(t *testing.T) {
	log.Init(io.Discard, false, false)
	d := &tunnel.Desc{
		Name:   "prod",
		Host:   "bastion",
		Status: tunnel.Closed,
		Forwards: []tunnel.Forward{
			{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432", Mode: tunnel.Local},
			{Name: "cache", LocalAddress: "6379", RemoteAddress: "redis:6379", Mode: tunnel.Local},
		},
	}
	out := stripANSI(tunnelTable([]*tunnel.Desc{d}).String())
	rows := nonEmptyLines(out)
	if len(rows) != 4 {
		t.Fatalf("two-forward tunnel must render header + tunnel row + 2 sub-rows, got %d: %q", len(rows), rows)
	}

	// Connection-level header row: status, name, host only.
	if got := strings.Fields(rows[1]); !equalSlice(got, []string{"closed", "prod", "bastion"}) {
		t.Errorf("tunnel header row = %v, want [closed prod bastion]", got)
	}

	// Forward sub-rows: indented branch glyphs and per-forward addresses.
	if !strings.Contains(rows[2], "├") || !strings.Contains(rows[3], "└") {
		t.Errorf("forward sub-rows missing ├/└ branches: %q / %q", rows[2], rows[3])
	}
	if got := strings.Fields(rows[2]); !equalSlice(got, []string{"├", "db", "5432", "->", "db:5432"}) {
		t.Errorf("first forward sub-row = %v", got)
	}
	if got := strings.Fields(rows[3]); !equalSlice(got, []string{"└", "cache", "6379", "->", "redis:6379"}) {
		t.Errorf("last forward sub-row = %v", got)
	}
}

// TestTunnelTableUnnamedForwardLabel proves an unnamed forward falls back to
// its local address as the sub-row label.
func TestTunnelTableUnnamedForwardLabel(t *testing.T) {
	log.Init(io.Discard, false, false)
	d := &tunnel.Desc{
		Name: "prod", Host: "bastion", Status: tunnel.Closed,
		Forwards: []tunnel.Forward{
			{LocalAddress: "8080", RemoteAddress: ":8080", Mode: tunnel.Local},
			{LocalAddress: "9090", RemoteAddress: ":9090", Mode: tunnel.Local},
		},
	}
	out := stripANSI(tunnelTable([]*tunnel.Desc{d}).String())
	if !strings.Contains(out, "├ 8080") {
		t.Errorf("unnamed forward did not use its local address as label:\n%s", out)
	}
}

// nonEmptyLines splits rendered output into lines, trimming trailing spaces
// and dropping blank lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimRight(l, " "); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// equalSlice reports whether two string slices are element-wise equal.
func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

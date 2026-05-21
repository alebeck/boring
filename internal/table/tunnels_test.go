package table

import (
	"regexp"
	"strings"
	"testing"
)

// strip removes ANSI escape sequences so assertions compare plain text.
func strip(s string) string {
	return ansi.ReplaceAllString(s, "")
}

// lines splits a rendered table into trimmed-of-trailing-space lines, dropping
// the leading and trailing blank lines String() produces.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	// Drop a trailing empty line if present.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func TestTunnelTableHeader(t *testing.T) {
	tbl := NewTunnelTable()
	got := strip(tbl.String())
	header := lines(got)[0]
	if want := []string{"Status", "Name", "Local", "Remote", "Via"}; !equalFields(header, want) {
		t.Fatalf("header = %q, want fields %v", header, want)
	}
}

// equalFields reports whether the whitespace-separated fields of line equal
// want.
func equalFields(line string, want []string) bool {
	got := strings.Fields(line)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestTunnelTableSingleForwardInline proves a single-forward tunnel renders on
// one line with status, name, the one forward's local/mode/remote, and the via
// host — no header/sub-row split.
func TestTunnelTableSingleForwardInline(t *testing.T) {
	tbl := NewTunnelTable()
	tbl.Add(TunnelRow{
		Status: "closed",
		Name:   "dev",
		Via:    "devhost",
		Forwards: []ForwardRow{
			{Label: "9000", Local: "9000", Mode: "->", Remote: ":9000"},
		},
	})
	got := lines(strip(tbl.String()))
	if len(got) != 2 {
		t.Fatalf("single-forward tunnel must render 2 lines (header + row), got %d: %q", len(got), got)
	}
	want := []string{"closed", "dev", "9000", "->", ":9000", "devhost"}
	if !equalFields(got[1], want) {
		t.Errorf("inline row fields = %v, want %v", strings.Fields(got[1]), want)
	}
}

// TestTunnelTableMultiForwardTree proves a multi-forward tunnel renders a
// connection-level header row (status, name, via — no forward columns)
// followed by one indented ├/└ sub-row per forward, each carrying the
// forward's label and local/mode/remote.
func TestTunnelTableMultiForwardTree(t *testing.T) {
	tbl := NewTunnelTable()
	tbl.Add(TunnelRow{
		Status: "open",
		Name:   "prod",
		Via:    "bastion",
		Forwards: []ForwardRow{
			{Label: "db", Local: "5432", Mode: "->", Remote: "db:5432"},
			{Label: "cache", Local: "6379", Mode: "->", Remote: "redis:6379"},
		},
	})
	got := lines(strip(tbl.String()))
	if len(got) != 4 {
		t.Fatalf("two-forward tunnel must render 4 lines (header + tunnel + 2 forwards), got %d: %q", len(got), got)
	}

	// The tunnel header row carries connection-level fields only.
	headerRow := got[1]
	if !equalFields(headerRow, []string{"open", "prod", "bastion"}) {
		t.Errorf("tunnel header row = %q, want fields [open prod bastion]", headerRow)
	}

	// First forward: ├ branch, label "db", local -> remote.
	first := got[2]
	if !strings.Contains(first, "├") {
		t.Errorf("first forward sub-row missing ├ branch: %q", first)
	}
	if !equalFields(first, []string{"├", "db", "5432", "->", "db:5432"}) {
		t.Errorf("first forward sub-row = %q", first)
	}

	// Last forward: └ branch, label "cache".
	last := got[3]
	if !strings.Contains(last, "└") {
		t.Errorf("last forward sub-row missing └ branch: %q", last)
	}
	if !equalFields(last, []string{"└", "cache", "6379", "->", "redis:6379"}) {
		t.Errorf("last forward sub-row = %q", last)
	}
}

// TestTunnelTableForwardColumnsAlign proves the Local column of an inline
// single-forward row and of a multi-forward sub-row start at the same screen
// column, so the grouped tree stays aligned.
func TestTunnelTableForwardColumnsAlign(t *testing.T) {
	tbl := NewTunnelTable()
	tbl.Add(TunnelRow{
		Status: "closed", Name: "dev", Via: "devhost",
		Forwards: []ForwardRow{{Label: "9000", Local: "9000", Mode: "->", Remote: ":9000"}},
	})
	tbl.Add(TunnelRow{
		Status: "open", Name: "prod", Via: "bastion",
		Forwards: []ForwardRow{
			{Label: "db", Local: "5432", Mode: "->", Remote: "db:5432"},
			{Label: "cache", Local: "6379", Mode: "->", Remote: "redis:6379"},
		},
	})
	got := lines(strip(tbl.String()))

	inline := got[1]       // closed dev ...
	sub := got[len(got)-1] // └ cache ...
	inlineLocal := visibleColumn(inline, "9000")
	subLocal := visibleColumn(sub, "6379")
	if inlineLocal < 0 || subLocal < 0 {
		t.Fatalf("could not locate Local values: inline=%q sub=%q", inline, sub)
	}
	if inlineLocal != subLocal {
		t.Errorf("Local column misaligned: inline at %d, sub-row at %d", inlineLocal, subLocal)
	}
}

// visibleColumn returns the printed column at which sub first appears in s,
// counting runes (not bytes) so multi-byte glyphs like ├/└ are one column wide.
func visibleColumn(s, sub string) int {
	byteIdx := strings.Index(s, sub)
	if byteIdx < 0 {
		return -1
	}
	return visibleWidth(s[:byteIdx])
}

// TestTunnelTableMultiForwardEmpty proves a tunnel with no forwards still
// renders its header line rather than panicking.
func TestTunnelTableMultiForwardEmpty(t *testing.T) {
	tbl := NewTunnelTable()
	tbl.Add(TunnelRow{Status: "closed", Name: "broken", Via: "host"})
	got := lines(strip(tbl.String()))
	if len(got) != 2 {
		t.Fatalf("forward-less tunnel must render 2 lines, got %d: %q", len(got), got)
	}
	if !equalFields(got[1], []string{"closed", "broken", "host"}) {
		t.Errorf("forward-less tunnel row = %q", got[1])
	}
}

// TestTunnelTableUnnamedForwardLabel proves an unnamed forward's Label (its
// local address) renders in the Name column of the sub-row.
func TestTunnelTableUnnamedForwardLabel(t *testing.T) {
	tbl := NewTunnelTable()
	tbl.Add(TunnelRow{
		Status: "open", Name: "prod", Via: "bastion",
		Forwards: []ForwardRow{
			{Label: "8080", Local: "8080", Mode: "->", Remote: ":8080"},
			{Label: "9090", Local: "9090", Mode: "->", Remote: ":9090"},
		},
	})
	got := strip(tbl.String())
	if !regexp.MustCompile(`├ 8080`).MatchString(got) {
		t.Errorf("unnamed forward did not render its local address as label:\n%s", got)
	}
}

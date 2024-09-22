package table

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	bold  = "\x1b[1m"
	reset = "\x1b[0m"
	pad   = 2
)

// Regex to match ANSI escape sequences
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type Table struct {
	header []string
	data   [][]string
	lens   []int
}

func New(cols ...string) *Table {
	lens := make([]int, len(cols))
	for i, c := range cols {
		lens[i] = length(c)
	}
	return &Table{header: cols, lens: lens}
}

func (t *Table) AddRow(cols ...any) {
	// TODO: only call ``length` once per entry
	if len(cols) != len(t.header) {
		panic("incorrect number of columns passed")
	}

	strs := make([]string, len(cols))
	for i, c := range cols {
		strs[i] = fmt.Sprintf("%v", c)
		t.lens[i] = max(t.lens[i], length(strs[i]))
	}

	t.data = append(t.data, strs)
}

func (t *Table) Print() {
	for j, h := range t.header {
		p := t.lens[j] + pad - length(h)
		fmt.Print(bold + h + reset + strings.Repeat(" ", p))
	}
	for _, row := range t.data {
		fmt.Print("\n")
		for j := range len(t.header) {
			p := t.lens[j] + pad - length(row[j])
			fmt.Printf(row[j] + strings.Repeat(" ", p))
		}
	}
	fmt.Print("\n")
}

func length(s string) int {
	s = ansi.ReplaceAllString(s, "")
	return len(s)
}
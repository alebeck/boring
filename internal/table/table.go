package table

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alebeck/boring/internal/log"
)

const pad = 2

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

func (t *Table) String() string {
	var accu string
	for j, h := range t.header {
		p := t.lens[j] + pad - length(h)
		accu += log.Bold + h + log.Reset + strings.Repeat(" ", p)
	}
	for _, row := range t.data {
		accu += "\n"
		for j := range len(t.header) {
			p := t.lens[j] + pad - length(row[j])
			accu += row[j] + strings.Repeat(" ", p)
		}
	}
	accu += "\n"
	return accu
}

func length(s string) int {
	s = ansi.ReplaceAllString(s, "")
	return len(s)
}

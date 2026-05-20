package tunnel

import "sort"

// Order combines configured and running tunnels into an ordered slice.
// Config order is preserved; where a configured tunnel is also running, the
// running copy is used. Running-but-not-configured tunnels are appended sorted
// by name. It is shared by the CLI list view and the TUI dashboard.
func Order(conf []Desc, running map[string]*Desc) []*Desc {
	var all []*Desc
	visited := make(map[string]bool)
	for i := range conf {
		t := &conf[i]
		if q, ok := running[t.Name]; ok {
			all = append(all, q)
			visited[q.Name] = true
			continue
		}
		all = append(all, t)
	}
	var extra []string
	for name := range running {
		if !visited[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		all = append(all, running[name])
	}
	return all
}

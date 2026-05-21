package tunnel

import (
	"reflect"
	"testing"
)

func TestOrder(t *testing.T) {
	conf := []Desc{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	runningB := &Desc{Name: "b"}
	running := map[string]*Desc{
		"b": runningB,    // configured AND running
		"z": {Name: "z"}, // running, not configured
		"x": {Name: "x"}, // running, not configured
	}
	got := Order(conf, running)

	var names []string
	for _, d := range got {
		names = append(names, d.Name)
	}
	want := []string{"a", "b", "c", "x", "z"} // config order, then extras sorted by name
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Order() names = %v, want %v", names, want)
	}
	// For a name that is both configured and running, the RUNNING copy is used.
	if got[1] != runningB {
		t.Fatal("Order must use the running tunnel for a configured+running name")
	}
}

func TestOrderEmpty(t *testing.T) {
	if got := Order(nil, nil); len(got) != 0 {
		t.Fatalf("Order(nil, nil) = %v, want empty", got)
	}
}

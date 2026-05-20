package tunnel

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestForwardModeMarshalsAsInt locks that Forward.Mode crosses the IPC socket
// as an integer, exactly like Desc.Mode. Mode has a MarshalTOML method but is
// deliberately not a TextMarshaler, so encoding/json must emit the numeric
// value.
func TestForwardModeMarshalsAsInt(t *testing.T) {
	f := Forward{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432", Mode: RemoteSocks}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"mode":3`) {
		t.Errorf("Mode must serialise as an integer; got %s", got)
	}
	if strings.Contains(got, `"socks-remote"`) {
		t.Errorf("Mode must not serialise as a string; got %s", got)
	}
}

// TestForwardJSONRoundTrip verifies a Forward survives a JSON round trip
// unchanged, including the zero-value (Local) mode.
func TestForwardJSONRoundTrip(t *testing.T) {
	cases := []Forward{
		{Name: "db", LocalAddress: "5432", RemoteAddress: "db:5432", Mode: Local},
		{LocalAddress: "9000", RemoteAddress: "localhost:9000", Mode: Remote},
		{Name: "proxy", Mode: Socks},
	}
	for _, want := range cases {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%+v): %v", want, err)
		}
		var got Forward
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", data, err)
		}
		if got != want {
			t.Errorf("round trip changed forward: got %+v, want %+v", got, want)
		}
	}
}

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

// TestForwardDisplayAddresses locks the render-time [SOCKS] derivation: a Socks
// forward shows the placeholder for its unused remote side, a RemoteSocks
// forward for its unused local side, and Local/Remote forwards show their plain
// addresses unchanged.
func TestForwardDisplayAddresses(t *testing.T) {
	cases := []struct {
		name       string
		forward    Forward
		wantLocal  string
		wantRemote string
	}{
		{
			name:       "local forward shows plain addresses",
			forward:    Forward{LocalAddress: "9000", RemoteAddress: "localhost:9000", Mode: Local},
			wantLocal:  "9000",
			wantRemote: "localhost:9000",
		},
		{
			name:       "remote forward shows plain addresses",
			forward:    Forward{LocalAddress: "localhost:8080", RemoteAddress: "8080", Mode: Remote},
			wantLocal:  "localhost:8080",
			wantRemote: "8080",
		},
		{
			name:       "socks forward shows label for the unused remote side",
			forward:    Forward{LocalAddress: "1080", Mode: Socks},
			wantLocal:  "1080",
			wantRemote: SocksLabel,
		},
		{
			name:       "socks-remote forward shows label for the unused local side",
			forward:    Forward{RemoteAddress: "1080", Mode: RemoteSocks},
			wantLocal:  SocksLabel,
			wantRemote: "1080",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.forward.DisplayLocal(); got != c.wantLocal {
				t.Errorf("DisplayLocal() = %q, want %q", got, c.wantLocal)
			}
			if got := c.forward.DisplayRemote(); got != c.wantRemote {
				t.Errorf("DisplayRemote() = %q, want %q", got, c.wantRemote)
			}
		})
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

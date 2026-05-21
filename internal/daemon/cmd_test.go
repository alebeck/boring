package daemon

import (
	"bufio"
	"encoding/json"
	"net"
	"reflect"
	"testing"

	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/tunnel"
)

// multiForwardDesc is a Desc with two forwards of differing modes, used to
// prove the multi-forward model survives the IPC encoding intact.
func multiForwardDesc() tunnel.Desc {
	return tunnel.Desc{
		Name: "prod",
		Host: "bastion",
		User: "deploy",
		Forwards: []tunnel.Forward{
			{
				Name:          "db",
				LocalAddress:  "5432",
				RemoteAddress: "db.internal:5432",
				Mode:          tunnel.Local,
			},
			{
				Name:          "metrics",
				LocalAddress:  "9090",
				RemoteAddress: "metrics.internal:9090",
				Mode:          tunnel.RemoteSocks,
			},
		},
	}
}

// TestOpenCmdMultiForwardRoundTrip proves a multi-forward Desc survives the
// CLI->daemon Open exchange: an Open Cmd encoded to JSON and read back over a
// connection preserves every forward (name/local/remote/mode).
func TestOpenCmdMultiForwardRoundTrip(t *testing.T) {
	want := Cmd{Kind: Open, Tunnel: multiForwardDesc()}

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	writeErr := make(chan error, 1)
	go func() { writeErr <- ipc.Write(want, b) }()

	var got Cmd
	if err := ipc.Read(&got, bufio.NewReader(a)); err != nil {
		t.Fatalf("ipc.Read: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("ipc.Write: %v", err)
	}

	if got.Kind != Open {
		t.Fatalf("Kind = %v, want Open", got.Kind)
	}
	if !reflect.DeepEqual(got.Tunnel.Forwards, want.Tunnel.Forwards) {
		t.Fatalf("Forwards round-trip mismatch:\n got %+v\nwant %+v",
			got.Tunnel.Forwards, want.Tunnel.Forwards)
	}
	if got.Tunnel.Name != want.Tunnel.Name || got.Tunnel.Host != want.Tunnel.Host {
		t.Fatalf("tunnel identity altered: got %+v", got.Tunnel)
	}
}

// TestListRespMultiForwardRoundTrip proves the daemon->CLI List response keeps
// each tunnel's Forwards intact: a Resp carrying a multi-forward Desc encodes
// to JSON and reads back with every forward preserved.
func TestListRespMultiForwardRoundTrip(t *testing.T) {
	desc := multiForwardDesc()
	desc.Status = tunnel.Open
	want := Resp{
		Success: true,
		Tunnels: map[string]tunnel.Desc{desc.Name: desc},
		Info:    Info{Commit: "abcde"},
	}

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	writeErr := make(chan error, 1)
	go func() { writeErr <- ipc.Write(want, b) }()

	var got Resp
	if err := ipc.Read(&got, bufio.NewReader(a)); err != nil {
		t.Fatalf("ipc.Read: %v", err)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("ipc.Write: %v", err)
	}

	if !got.Success {
		t.Fatalf("Success = false, want true")
	}
	gotDesc, ok := got.Tunnels[desc.Name]
	if !ok {
		t.Fatalf("tunnel %q missing from response: %+v", desc.Name, got.Tunnels)
	}
	if !reflect.DeepEqual(gotDesc.Forwards, desc.Forwards) {
		t.Fatalf("Forwards round-trip mismatch:\n got %+v\nwant %+v",
			gotDesc.Forwards, desc.Forwards)
	}
	if gotDesc.Status != tunnel.Open {
		t.Fatalf("Status = %v, want Open", gotDesc.Status)
	}
}

// TestDescOmitsLegacyAddressFields locks the IPC contract: the legacy
// single-forward shorthand fields (Desc.LocalAddress/RemoteAddress/Mode) carry
// json:"-" and so never reach the wire. Forwards alone carries the per-forward
// data over the socket. The legacy "local"/"remote"/"mode" keys must be absent
// from a marshalled Desc even when the legacy fields are populated.
func TestDescOmitsLegacyAddressFields(t *testing.T) {
	desc := multiForwardDesc()
	// Populate the legacy fields to prove json:"-" excludes them regardless of
	// value: a real config.Load never sets them on a multi-forward tunnel, but
	// the IPC exclusion must not depend on that.
	desc.LocalAddress = "5432"
	desc.RemoteAddress = "db.internal:5432"
	desc.Mode = tunnel.Local

	data, err := json.Marshal(desc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Decode into a generic map to assert on the actual top-level Desc keys.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"local", "remote", "mode"} {
		if _, ok := fields[key]; ok {
			t.Errorf("legacy key %q present in marshalled Desc: %s", key, data)
		}
	}
	if _, ok := fields["forwards"]; !ok {
		t.Errorf("forwards key missing from marshalled Desc: %s", data)
	}

	// A multi-forward Desc still round-trips with its Forwards intact.
	var got tunnel.Desc
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal into Desc: %v", err)
	}
	if !reflect.DeepEqual(got.Forwards, desc.Forwards) {
		t.Fatalf("Forwards round-trip mismatch:\n got %+v\nwant %+v",
			got.Forwards, desc.Forwards)
	}
	// The legacy fields do not survive the wire; consumers must read Forwards.
	if got.LocalAddress != "" || got.RemoteAddress != "" || got.Mode != tunnel.Local {
		// Mode's zero value is Local, so an unmarshalled-from-wire Desc has
		// Mode == Local regardless; only the addresses prove the exclusion.
		if got.LocalAddress != "" || got.RemoteAddress != "" {
			t.Errorf("legacy address fields survived the wire: local=%q remote=%q",
				got.LocalAddress, got.RemoteAddress)
		}
	}
}

// TestSingleForwardDescOmitsLegacyKeys mirrors the multi-forward case for a
// single-forward Desc, the common shape: a legacy config loads into exactly
// one forward and that forward is what crosses the socket.
func TestSingleForwardDescOmitsLegacyKeys(t *testing.T) {
	desc := tunnel.Desc{
		Name: "dev",
		Host: "devhost",
		// Legacy fields populated as config.Load would leave them for a
		// legacy-shorthand tunnel.
		LocalAddress:  "9000",
		RemoteAddress: "localhost:9000",
		Mode:          tunnel.Local,
		Forwards: []tunnel.Forward{{
			LocalAddress:  "9000",
			RemoteAddress: "localhost:9000",
			Mode:          tunnel.Local,
		}},
	}

	data, err := json.Marshal(desc)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// Decode into a generic map: Forward also uses json "local"/"remote"/"mode"
	// keys, so a substring scan would false-positive on the nested forward.
	// Only the top-level Desc keys must be checked.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal into map: %v", err)
	}
	for _, key := range []string{"local", "remote", "mode"} {
		if _, ok := fields[key]; ok {
			t.Errorf("legacy key %q present in marshalled Desc: %s", key, data)
		}
	}

	var got tunnel.Desc
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got.Forwards, desc.Forwards) {
		t.Fatalf("Forwards round-trip mismatch:\n got %+v\nwant %+v",
			got.Forwards, desc.Forwards)
	}
}

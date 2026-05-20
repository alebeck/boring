package daemon

import (
	"bufio"
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

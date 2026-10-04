package daemon

import (
	"testing"

	"github.com/alebeck/boring/internal/tunnel"
)

func TestRemoveTunnelKeepsNewer(t *testing.T) {
	d := &daemon{tunnels: make(map[string]*tunnel.Tunnel)}
	old := tunnel.FromDesc(&tunnel.Desc{Name: "a"})
	cur := tunnel.FromDesc(&tunnel.Desc{Name: "a"})
	d.tunnels["a"] = cur

	d.removeTunnel(old)
	if d.tunnels["a"] != cur {
		t.Fatal("removing an old tunnel dropped the one that replaced it")
	}

	d.removeTunnel(cur)
	if _, ok := d.tunnels["a"]; ok {
		t.Fatal("tunnel not removed")
	}
}

package tunnel

import (
	"fmt"
	"net"
)

// forwardRuntime is the per-forward runtime state of a Tunnel. A tunnel owns a
// single SSH connection and one forwardRuntime per Forward: each carries its
// own listener and the parsed local/remote addresses for its own mode.
type forwardRuntime struct {
	forward    Forward
	listener   net.Listener
	localAddr  *address
	remoteAddr *address
}

// label names a forward for error messages and logs, delegating to the
// underlying Forward.
func (f *forwardRuntime) label() string {
	return f.forward.label()
}

// parseForward builds a forwardRuntime from a Forward, parsing its local and
// remote addresses per the forward's own mode. The address rules mirror the
// legacy single-forward logic: remote/socks-remote forwards accept a bare port
// on the remote side, local/socks forwards accept one on the local side.
func parseForward(f Forward) (*forwardRuntime, error) {
	allowShort := f.Mode == Remote || f.Mode == RemoteSocks

	remoteAddr, err := parseAddr(string(f.RemoteAddress), allowShort)
	if err != nil {
		return nil, fmt.Errorf("remote address: %v", err)
	}

	localAddr, err := parseAddr(string(f.LocalAddress), !allowShort)
	if err != nil {
		return nil, fmt.Errorf("local address: %v", err)
	}

	return &forwardRuntime{
		forward:    f,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}, nil
}

// isRemote reports whether the forward's mode listens on the remote side.
func (f *forwardRuntime) isRemote() bool {
	return f.forward.Mode == Remote || f.forward.Mode == RemoteSocks
}

// isSocks reports whether the forward's mode runs a SOCKS5 proxy.
func (f *forwardRuntime) isSocks() bool {
	return f.forward.Mode == Socks || f.forward.Mode == RemoteSocks
}

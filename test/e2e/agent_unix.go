//go:build !windows

package e2e

import "net"

func listenAgent(sock string) (net.Listener, error) {
	return net.Listen("unix", sock)
}

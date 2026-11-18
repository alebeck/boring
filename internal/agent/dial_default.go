//go:build !windows

package agent

import (
	"io"
	"net"
)

const defaultSock = ""

func dial(sock string) (io.ReadWriteCloser, error) {
	return net.Dial("unix", sock)
}

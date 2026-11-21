package e2e

import (
	"net"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

func listenAgent(sock string) (net.Listener, error) {
	if !strings.HasPrefix(sock, `\\.\pipe\`) {
		return net.Listen("unix", sock)
	}
	name, err := windows.UTF16PtrFromString(sock)
	if err != nil {
		return nil, err
	}
	l := &pipeListener{name: name}
	if l.next, err = l.newInstance(); err != nil {
		return nil, err
	}
	return l, nil
}

type pipeListener struct {
	name *uint16
	next windows.Handle
	// Must not move while a client is awaited, so it cannot be on the stack
	ov     windows.Overlapped
	closed atomic.Bool
}

func (l *pipeListener) newInstance() (windows.Handle, error) {
	return windows.CreateNamedPipe(l.name,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE, windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, nil)
}

func (l *pipeListener) Accept() (net.Conn, error) {
	h := l.next
	l.ov = windows.Overlapped{}
	err := windows.ConnectNamedPipe(h, &l.ov)
	if err == windows.ERROR_IO_PENDING {
		var n uint32
		err = windows.GetOverlappedResult(h, &l.ov, &n, true)
	}
	if err == nil || err == windows.ERROR_PIPE_CONNECTED {
		if l.next, err = l.newInstance(); err == nil && l.closed.Load() {
			windows.CloseHandle(l.next)
			err = net.ErrClosed
		}
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return pipeConn{os.NewFile(uintptr(h), "pipe")}, nil
}

func (l *pipeListener) Close() error {
	l.closed.Store(true)
	// Accept only returns once a client connects, so do that
	h, err := windows.CreateFile(l.name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}

func (l *pipeListener) Addr() net.Addr { return nil }

type pipeConn struct{ *os.File }

func (pipeConn) LocalAddr() net.Addr  { return nil }
func (pipeConn) RemoteAddr() net.Addr { return nil }

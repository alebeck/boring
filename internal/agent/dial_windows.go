//go:build windows

package agent

import (
	"io"
	"net"
	"os"
	"strings"
	"syscall"
)

// defaultSock is the named pipe Windows OpenSSH uses if SSH_AUTH_SOCK is not
// set. Third-party agents such as 1Password listen on it as well.
const defaultSock = `\\.\pipe\openssh-ssh-agent`

func dial(sock string) (io.ReadWriteCloser, error) {
	if !strings.HasPrefix(strings.ToLower(strings.ReplaceAll(sock, "/", `\`)), `\\.\pipe\`) {
		return net.Dial("unix", sock)
	}
	// TODO: once on Go 1.26, replace the code below with
	// os.OpenFile(sock, os.O_RDWR|syscall.FILE_FLAG_OVERLAPPED, 0)
	name, err := syscall.UTF16PtrFromString(sock)
	if err != nil {
		return nil, err
	}
	// Open for overlapped I/O, so the handle is served by the runtime poller
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: sock, Err: err}
	}
	return os.NewFile(uintptr(h), sock), nil
}

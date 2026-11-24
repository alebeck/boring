package e2e

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/alebeck/boring/internal/daemon"
)

func pidRunning(pid int) bool {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	// The handle is only signaled once the process has exited
	ev, _ := syscall.WaitForSingleObject(h, 0)
	return ev == syscall.WAIT_TIMEOUT
}

func stopDaemon(_ *os.Process, sock string) error {
	// Windows cannot signal processes, so send the shutdown command
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(daemon.Cmd{Kind: daemon.Shutdown}); err != nil {
		return err
	}
	// The daemon closes the connection after responding
	_, err = io.Copy(io.Discard, conn)
	return err
}

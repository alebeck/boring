package tui

import (
	"fmt"
	"net"
	"time"

	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/tunnel"
	tea "github.com/charmbracelet/bubbletea"
)

// pollTimeout bounds a single daemon poll so a stuck daemon cannot leak
// goroutines or freeze the UI.
const pollTimeout = 5 * time.Second

// tunnelsMsg carries the result of one daemon List poll.
type tunnelsMsg struct {
	running map[string]*tunnel.Desc
	err     error
}

// pollTick is emitted by the poll timer to trigger the next poll.
type pollTick struct{}

// pollTunnels returns a command that asks the daemon for the running tunnels.
func pollTunnels() tea.Cmd {
	return func() tea.Msg {
		running, err := listRunning()
		return tunnelsMsg{running: running, err: err}
	}
}

// listRunning dials the daemon and returns the currently running tunnels.
func listRunning() (map[string]*tunnel.Desc, error) {
	conn, err := net.DialTimeout("unix", daemon.Socket, pollTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not reach daemon: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(pollTimeout))
	if err := ipc.Write(daemon.Cmd{Kind: daemon.List}, conn); err != nil {
		return nil, fmt.Errorf("could not send list command: %w", err)
	}
	var resp daemon.Resp
	if err := ipc.Read(&resp, conn); err != nil {
		return nil, fmt.Errorf("could not read daemon response: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	m := make(map[string]*tunnel.Desc, len(resp.Tunnels))
	for name := range resp.Tunnels {
		d := resp.Tunnels[name]
		m[name] = &d
	}
	return m, nil
}

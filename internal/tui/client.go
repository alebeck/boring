package tui

import (
	"fmt"
	"net"
	"time"

	"github.com/alebeck/boring/internal/auth"
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

// actionResultMsg is the outcome of an open or close action.
type actionResultMsg struct {
	verb string // "Opened" or "Closed"
	name string
	err  error
}

// openTunnelCmd returns a command that asks the daemon to open desc, using
// prompter for any interactive auth.
func openTunnelCmd(desc tunnel.Desc, prompter auth.Prompter) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("unix", daemon.Socket, pollTimeout)
		if err != nil {
			return actionResultMsg{verb: "Opened", name: desc.Name, err: err}
		}
		defer conn.Close()
		// No SetDeadline here: an interactive open blocks on the user typing
		// a 2FA code / passphrase, which is human-paced. A clean quit is
		// handled instead by draining pending auth requests (abortAllAuth).
		resp, err := daemon.RunOpenExchange(conn, daemon.Cmd{Kind: daemon.Open, Tunnel: desc}, prompter)
		if err == nil && resp != nil && !resp.Success {
			err = fmt.Errorf("%s", resp.Error)
		}
		return actionResultMsg{verb: "Opened", name: desc.Name, err: err}
	}
}

// closeTunnelCmd returns a command that asks the daemon to close the named tunnel.
func closeTunnelCmd(name string) tea.Cmd {
	return func() tea.Msg {
		conn, err := net.DialTimeout("unix", daemon.Socket, pollTimeout)
		if err != nil {
			return actionResultMsg{verb: "Closed", name: name, err: err}
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(pollTimeout))
		if err := ipc.Write(daemon.Cmd{Kind: daemon.Close, Tunnel: tunnel.Desc{Name: name}}, conn); err != nil {
			return actionResultMsg{verb: "Closed", name: name, err: err}
		}
		var resp daemon.Resp
		if err := ipc.Read(&resp, conn); err != nil {
			return actionResultMsg{verb: "Closed", name: name, err: err}
		}
		if !resp.Success {
			return actionResultMsg{verb: "Closed", name: name, err: fmt.Errorf("%s", resp.Error)}
		}
		return actionResultMsg{verb: "Closed", name: name}
	}
}

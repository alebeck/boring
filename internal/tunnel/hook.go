package tunnel

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

//
// Example hook to send an email when a tunnel disconnects:
// on_disconnect = "echo 'Tunnel disconnected' | mail -s 'Alert' user@example.com"
//

type Hook int

const (
	onConnect Hook = iota
)

const hookTimeout = 1 * time.Minute

func (t *Tunnel) runHook(h Hook) error {
	var command string
	switch h {
	case onConnect:
		command = t.OnConnect
	default:
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Env = append(os.Environ(),
		"BORING_NAME="+t.Name,
		"BORING_LOCAL="+string(t.LocalAddress),
		"BORING_REMOTE="+string(t.RemoteAddress),
		"BORING_HOST="+t.Host,
		"BORING_USER="+t.User,
	)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	if err != nil {
		return err
	}

	// TODO: distinguish ExitError from exec error
}

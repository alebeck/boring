//
// Logic for launching and controlling the Boring daemon.
//

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/alebeck/boring/internal/buildinfo"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
)

var doNotSpawn = os.Getenv("BORING_NO_SPAWN") != ""

// compatError indicates an error due to incompatible daemon version
type compatError struct {
	daemonHash string
	cliHash    string
}

func (e *compatError) Error() string {
	return fmt.Sprintf("daemon version %s not compatible with cli version %s",
		e.daemonHash, e.cliHash)
}

// connectDaemon connects to the daemon on the default socket
func connectDaemon() (net.Conn, error) {
	return net.Dial("unix", daemon.Socket)
}

// ensureDaemon starts the daemon if not already running, provided
// that the BORING_NO_SPAWN environment variable is not set. It will
// also check if the daemon is compatible with the CLI binary, and
// relaunch it if necessary.
func ensureDaemon(ctx context.Context) error {
	launching := false
	wait := time.NewTimer(0)
	waitTime := 4 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait.C:
			err := probeDaemon()
			if err == nil {
				return nil
			}
			var ce *compatError
			if errors.As(err, &ce) {
				b := "unknown daemon build"
				if ce.daemonHash != "" {
					b = fmt.Sprintf("daemon build %s#%s%s",
						log.Yellow, ce.daemonHash, log.Reset)
				}
				log.Infof("Detected %s (CLI: %s#%s%s), restarting daemon...",
					b, log.Green, ce.cliHash, log.Reset)
				// Terminate and wait for restart
				if err := killDaemon(ctx); err != nil {
					info := "Please kill the old daemon process manually. This will be" +
						" automatic from now on."
					return fmt.Errorf("could not kill old daemon: %v. %v", err,
						log.Bold+info+log.Reset)
				}
			}

			// Daemon is not yet running
			if doNotSpawn {
				return fmt.Errorf("not running and BORING_NO_SPAWN is set")
			}
			if !launching {
				if err := launchDaemon(); err != nil {
					return fmt.Errorf("launch daemon: %v", err)
				}
				launching = true
			}
			wait.Reset(waitTime)
			waitTime *= 2
		}
	}
}

func sendCmd(cmd daemon.Cmd) (*daemon.Resp, error) {
	conn, err := connectDaemon()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var resp daemon.Resp
	if err := ipc.Write(cmd, conn); err != nil {
		return nil, err
	}
	if err := ipc.Read(&resp, conn); err != nil {
		return nil, err
	}

	return &resp, nil
}

// probeDaemon checks whether a daemon on the default socket is responsive,
// and it's commit hash matches the calling binary's.
func probeDaemon() error {
	resp, err := sendCmd(daemon.Cmd{Kind: daemon.Nop})
	if err != nil {
		return err
	}
	// If the CLI binary is not built with a commit hash, don't check compatibility.
	// This increases robustness in some non-production scenarios.
	if buildinfo.Commit == "" {
		return nil
	}
	if resp.Info.Commit == "" || resp.Info.Commit != buildinfo.Commit {
		return &compatError{daemonHash: resp.Info.Commit, cliHash: buildinfo.Commit}
	}
	return nil
}

// killDaemon sends a shutdown command to the daemon and waits for it to exit
func killDaemon(ctx context.Context) error {
	resp, err := sendCmd(daemon.Cmd{Kind: daemon.Shutdown})
	if err != nil {
		return fmt.Errorf("could not send shutdown command: %v", err)
	}
	if !resp.Success {
		return fmt.Errorf("daemon error: %s", resp.Error)
	}

	// Wait for termination
	wait := 20 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			if ln, err := net.Listen("unix", daemon.Socket); err == nil {
				// We can bind, must be free
				ln.Close()
				return nil
			}
			wait *= 2
		}
	}
}

// launchDaemon starts a new daemon process, invoking the OS-specific launch function.
func launchDaemon() error {
	ex, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %v", err)
	}
	pid, err := launchDaemonOS(ex, daemon.Flag)
	if err != nil {
		return err
	}
	log.Debugf("Daemon started with PID %d", pid)
	return nil
}

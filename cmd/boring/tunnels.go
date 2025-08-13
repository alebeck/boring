package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"
	"golang.org/x/sync/errgroup"
)

const daemonTimeout = 2 * time.Second

var errOpFailed = errors.New("operation failed")

// prepare loads the configuration and ensures the daemon is running
func prepare() (*config.Config, error) {
	var conf *config.Config
	ctx, cancel := context.WithTimeout(context.Background(), daemonTimeout)
	g, ctx := errgroup.WithContext(ctx)
	defer cancel()

	g.Go(func() error {
		var err error
		if isTerm {
			if err = ensureConfig(); err != nil {
				return fmt.Errorf("could not create config file: %v", err)
			}
		}
		if conf, err = config.Load(); err != nil {
			if errors.Is(err, fs.ErrNotExist) && !isTerm {
				conf = &config.Config{}
				return nil
			}
			return fmt.Errorf("could not load config: %v", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := daemon.Ensure(ctx); err != nil {
			return fmt.Errorf("could not start daemon: %v", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return conf, nil
}

func controlTunnels(args []string, kind daemon.CmdKind) {
	if args[0] == "--all" || args[0] == "-a" {
		if len(args) != 1 {
			log.Fatalf("'--all' does not take any additional arguments.")
		}
		args = []string{"*"}
	}

	conf, err := prepare()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}

	// Get available tunnels for requested command
	tunnels := conf.TunnelsMap
	if kind == daemon.Close {
		tunnels, err = getRunningTunnels()
		if err != nil {
			log.Fatalf("Could not get running tunnels: %v", err)
		}
	}

	// Filter tunnels based on patterns
	keep := make(map[string]bool, len(tunnels))
	var notMatched []string
	for _, pat := range args {
		n, err := filterGlob(tunnels, keep, pat)
		if err != nil {
			log.Fatalf("Malformed glob pattern '%v'.", pat)
		}
		if n == 0 {
			notMatched = append(notMatched, pat)
		}
	}

	var m string
	if kind == daemon.Close {
		m = "running "
	}
	if len(keep) == 0 {
		// No tunnels to operate on, print error message
		msg := fmt.Sprintf("No %stunnels match pattern '%s'.", m, args[0])
		if len(args) > 1 {
			msg = fmt.Sprintf("No %stunnels match any provided pattern.", m)
		}
		log.Fatalf("%s", msg)
	}

	// If tunnels were matched, do print a warning for unmatched patterns
	for _, pat := range notMatched {
		log.Warningf("No %stunnels match pattern '%s'.", m, pat)
	}

	// Issue concurrent commands for all tunnels
	var g errgroup.Group
	for n := range keep {
		g.Go(func() error {
			if kind == daemon.Open {
				return openTunnel(tunnels[n])
			} else if kind == daemon.Close {
				return closeTunnel(tunnels[n])
			}
			panic("unknown command kind: " + kind.String())
		})
	}
	// This is just for determining the exit code really,
	// a detailed message will have been logged to the user.
	if err := g.Wait(); err != nil {
		os.Exit(1)
	}
}

func openTunnel(t *tunnel.Desc) error {
	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Open, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'open' command: %v", err)
		return errOpFailed
	}

	if !resp.Success {
		// cannot use errors.Is because error is transmitted as string over IPC
		if strings.HasSuffix(resp.Error, daemon.AlreadyRunning.Error()) {
			log.Infof("Tunnel '%v' is already running.", t.Name)
			return nil
		}
		log.Errorf("Could not open tunnel '%v': %v", t.Name, resp.Error)
		return errOpFailed
	}

	log.Infof("Opened tunnel '%s': %s %v %s via %s.", log.Green+log.Bold+t.Name+log.Reset,
		t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	return nil
}

func closeTunnel(t *tunnel.Desc) error {
	// Daemon only needs the name, so simplify
	t = &tunnel.Desc{Name: t.Name}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'close' command: %v", err)
		return errOpFailed
	}

	if !resp.Success {
		log.Errorf("Tunnel '%v' could not be closed: %v", t.Name, resp.Error)
		return errOpFailed
	}
	log.Infof("Closed tunnel '%s'.", log.Green+log.Bold+t.Name+log.Reset)
	return nil
}

func getRunningTunnels() (map[string]*tunnel.Desc, error) {
	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.List}
	if err := transmitCmd(cmd, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	m := make(map[string]*tunnel.Desc, len(resp.Tunnels))
	for _, t := range resp.Tunnels {
		m[t.Name] = &t
	}
	return m, nil
}

func listTunnels() {
	conf, err := prepare()
	if err != nil {
		log.Fatalf("%s", err.Error())
	}

	ts, err := getRunningTunnels()
	if err != nil {
		log.Fatalf("Could not list tunnels: %v", err)
	}

	if len(ts) == 0 && len(conf.Tunnels) == 0 {
		log.Infof("No tunnels configured.")
		return
	}

	tbl := table.New("Status", "Name", "Local", "", "Remote", "Via")
	visited := make(map[string]bool)

	for _, t := range conf.Tunnels {
		if q, ok := ts[t.Name]; ok {
			tbl.AddRow(status(q), q.Name, q.LocalAddress, q.Mode, q.RemoteAddress, q.Host)
			visited[q.Name] = true
			continue
		}
		// TODO: case where tunnel is in resp but with different name
		tbl.AddRow(status(&t), t.Name, t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	}

	// Add tunnels that are in resp but not in the config
	for _, q := range ts {
		if !visited[q.Name] {
			tbl.AddRow(status(q), q.Name, q.LocalAddress, q.Mode, q.RemoteAddress, q.Host)
		}
	}

	log.Emitf("%v", tbl)
}

func transmitCmd(cmd daemon.Cmd, resp any) error {
	conn, err := daemon.Connect()
	if err != nil {
		return fmt.Errorf("could not connect to daemon: %v", err)
	}
	defer conn.Close()

	if err := ipc.Send(cmd, conn); err != nil {
		return fmt.Errorf("could not send command: %v", err)
	}

	if err = ipc.Receive(resp, conn); err != nil {
		return fmt.Errorf("could not receive response: %v", err)
	}

	return nil
}

func filterGlob(
	ts map[string]*tunnel.Desc, keep map[string]bool, pat string) (
	n int, err error) {
	// Fail early if pattern is malformed; if this passes we can
	// ignore the error return value of the following matches
	if _, err = filepath.Match(pat, ""); err != nil {
		return
	}
	for t := range ts {
		if m, _ := filepath.Match(pat, t); m {
			keep[t] = true
			n++
		}
	}
	return
}

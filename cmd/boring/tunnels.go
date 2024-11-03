package main

import (
	"context"
	"fmt"
	"path/filepath"
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

// prepare loads the configuration and ensures the daemon is running
func prepare() (*config.Config, error) {
	var conf *config.Config
	ctx, cancel := context.WithTimeout(context.Background(), daemonTimeout)
	g, ctx := errgroup.WithContext(ctx)
	defer cancel()

	g.Go(func() error {
		var err error
		if err = ensureConfig(); err != nil {
			return fmt.Errorf("could not create config file: %v", err)
		}
		if conf, err = config.Load(); err != nil {
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
	var glob string
	if args[0] == "--all" || args[0] == "-a" {
		if len(args) != 1 {
			log.Fatalf("'--all' does not take any additional arguments.")
		}
		glob = "*"
	} else if args[0] == "--glob" || args[0] == "-g" {
		if len(args) != 2 {
			log.Fatalf("'--glob' takes exactly one argument.")
		}
		glob = args[1] // `args[1]` is a glob pattern
	}

	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Get available tunnels for requested command
	tunnels := conf.TunnelsMap
	if kind == daemon.Close {
		tunnels, err = getRunningTunnels()
		if err != nil {
			log.Fatalf("Could not get running tunnels: %v", err)
		}
	}

	// Filter tunnels based on user arguments
	if glob == "" {
		// `args` are plain tunnel names
		extra := filterNames(tunnels, args)
		for _, n := range extra {
			msg := fmt.Sprintf("Tunnel '%s' not found in configuration.", n)
			if kind == daemon.Close {
				msg = fmt.Sprintf("Tunnel '%s' is not running.", n)
			}
			log.Errorf(msg)
		}
	} else {
		if err = filterGlob(tunnels, glob); err != nil {
			log.Fatalf("Malformed glob pattern: %v", args[1])
		}
		if len(tunnels) == 0 {
			msg := fmt.Sprintf("No tunnels match pattern '%s'.", glob)
			if kind == daemon.Close {
				msg = fmt.Sprintf("No running tunnels match pattern '%s'.", glob)
			}
			log.Errorf(msg)
		}
	}

	// Issue concurrent commands for all tunnels
	done := make(chan struct{}, len(tunnels))
	for _, t := range tunnels {
		go func() {
			if kind == daemon.Open {
				openTunnel(t)
			} else if kind == daemon.Close {
				closeTunnel(t)
			}
			done <- struct{}{}
		}()
	}

	for range tunnels {
		<-done
	}
}

func openTunnel(t *tunnel.Tunnel) {
	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Open, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'open' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel '%v' could not be opened: %v", t.Name, resp.Error)
	} else {
		log.Infof("Opened tunnel '%s': %s %v %s via %s", log.Green+t.Name+log.Reset,
			t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	}
}

func closeTunnel(t *tunnel.Tunnel) {
	// The daemon only needs the name for closing, so simplify
	t = &tunnel.Tunnel{Name: t.Name}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'close' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel '%v' could not be closed: %v", t.Name, resp.Error)
	} else {
		log.Infof("Closed tunnel '%s'", log.Green+t.Name+log.Reset)
	}
}

func getRunningTunnels() (map[string]*tunnel.Tunnel, error) {
	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.List}
	if err := transmitCmd(cmd, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf(resp.Error)
	}
	m := make(map[string]*tunnel.Tunnel, len(resp.Tunnels))
	for _, t := range resp.Tunnels {
		m[t.Name] = &t
	}
	return m, nil
}

func listTunnels() {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
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

	tbl.Print()
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

func filterGlob(ts map[string]*tunnel.Tunnel, pat string) (err error) {
	// Fail early if pattern is malformed; if this passes we can
	// ignore the error return value of the following matches
	if _, err = filepath.Match(pat, ""); err != nil {
		return
	}
	for n := range ts {
		if m, _ := filepath.Match(pat, n); !m {
			delete(ts, n)
		}
	}
	return
}

func filterNames(ts map[string]*tunnel.Tunnel, names []string) (extra []string) {
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		if _, ok := ts[n]; ok {
			keep[n] = true
			continue
		}
		extra = append(extra, n)
	}
	for n := range ts {
		if !keep[n] {
			delete(ts, n)
		}
	}
	return
}

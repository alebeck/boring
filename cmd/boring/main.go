package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
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

var version, commit string

func main() {
	if len(os.Args) == 2 && os.Args[1] == daemon.Flag {
		// Run in daemon mode
		daemon.Run()
		os.Exit(0)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "open", "o":
		if len(os.Args) < 3 {
			log.Fatalf("'open' requires at least one 'name' argument.")
		}
		controlTunnels(os.Args[2:], daemon.Open)
	case "close", "c":
		if len(os.Args) < 3 {
			log.Fatalf("'close' requires at least one 'name' argument.")
		}
		controlTunnels(os.Args[2:], daemon.Close)
	case "list", "l":
		listTunnels()
	case "edit", "e":
		openConfig()
	default:
		fmt.Println("Unknown command:", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// prepare loads the configuration and ensures the daemon is running
func prepare() (*config.Config, error) {
	var conf *config.Config
	ctx, cancel := context.WithTimeout(context.Background(), daemonTimeout)
	g, ctx := errgroup.WithContext(ctx)
	defer cancel()

	g.Go(func() error {
		var err error
		// Makes sure config file exists, and otherwise creates it
		if err := config.Ensure(); err != nil {
			return fmt.Errorf("could not create config file: %v", err)
		}
		if conf, err = config.Load(); err != nil {
			return fmt.Errorf("could not load configuration: %v", err)
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

func controlTunnels(names []string, kind daemon.CmdKind) {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Remove potential duplicates from names list
	names = slices.Compact(slices.Sorted(slices.Values(names)))

	// Issue concurrent commands for all tunnels
	done := make(chan bool, len(names))
	for _, name := range names {
		go func() {
			if kind == daemon.Open {
				openTunnel(name, conf)
			} else if kind == daemon.Close {
				closeTunnel(name)
			} else {
				log.Errorf("Unknown command: %v", kind)
			}
			done <- true
		}()
	}

	for range names {
		<-done
	}
}

func openTunnel(name string, conf *config.Config) {
	t, ok := conf.TunnelsMap[name]
	if !ok {
		log.Errorf("Tunnel '%s' not found in configuration (%s).",
			name, config.FilePath)
		return
	}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Open, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'open' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel '%v' could not be opened: %v", name, resp.Error)
	} else {
		log.Infof("Opened tunnel '%s': %s %v %s via %s",
			log.Green+t.Name+log.Reset,
			t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	}
}

func closeTunnel(name string) {
	// The daemon only needs the name for closing. In cases where the
	// config has changed, the name is all we have about the tunnel anyway.
	t := tunnel.Tunnel{Name: name}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'close' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel '%v' could not be closed: %v", name, resp.Error)
	} else {
		log.Infof("Closed tunnel '%s'", log.Green+t.Name+log.Reset)
	}
}

func listTunnels() {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
		return
	}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.List}
	if err = transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit command: %v", err)
		return
	}
	if !resp.Success {
		log.Errorf("Could not list tunnels: %v", resp.Error)
		return
	}

	if len(resp.Tunnels) == 0 && len(conf.Tunnels) == 0 {
		log.Infof("No tunnels configured.")
		return
	}

	tbl := table.New("Status", "Name", "Local", "", "Remote", "Via")
	visited := make(map[string]bool)

	for _, t := range conf.Tunnels {
		if q, ok := resp.Tunnels[t.Name]; ok {
			tbl.AddRow(q.Status, q.Name, q.LocalAddress, q.Mode, q.RemoteAddress, q.Host)
			visited[q.Name] = true
			continue
		}
		// TODO: case where tunnel is in resp but with different name
		tbl.AddRow(tunnel.Closed, t.Name, t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	}

	// Add tunnels that are in resp but not in the config
	for _, q := range resp.Tunnels {
		if !visited[q.Name] {
			tbl.AddRow(q.Status, q.Name, q.LocalAddress, q.Mode, q.RemoteAddress, q.Host)
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

func openConfig() {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
		if runtime.GOOS == "windows" {
			editor = "notepad"
		}
	}

	cmd := exec.Command(editor, config.FilePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("Editor: %v", err)
	}
}

func printUsage() {
	v := version
	if v == "" {
		v = "snapshot"
		if commit != "" {
			v += fmt.Sprintf(" (#%s)", commit)
		}
	}

	fmt.Printf("boring %s\n", v)
	fmt.Println("Usage:")
	fmt.Println("  boring list,l                         List tunnels")
	fmt.Println("  boring open,o <name1> [<name2> ...]   Open specified tunnel(s)")
	fmt.Println("  boring close,c <name1> [<name2> ...]  Close specified tunnel(s)")
	fmt.Println("  boring edit,e                         Edit configuration file")
}

package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"
)

func main() {
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
	default:
		fmt.Println("Unknown command:", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// prepare loads the configuration and ensures the daemon is running
func prepare() (*config.Config, error) {
	var conf *config.Config
	errs := make(chan error, 2)

	go func() {
		var err error
		conf, err = config.LoadConfig()
		if err != nil {
			err = fmt.Errorf("Could not load configuration: %v", err)
		}
		errs <- err
	}()

	go func() {
		err := daemon.Ensure()
		if err != nil {
			err = fmt.Errorf("Could not start daemon: %v", err)
		}
		errs <- err
	}()

	for _ = range 2 {
		if err := <-errs; err != nil {
			return nil, err
		}
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
			controlTunnel(name, kind, conf)
			done <- true
		}()
	}

	for _ = range names {
		<-done
	}
}

func controlTunnel(name string, kind daemon.CmdKind, conf *config.Config) {
	t, ok := conf.TunnelsMap[name]
	if !ok {
		log.Errorf("Tunnel '%s' not found in configuration (%s).",
			name, config.CONFIG_FILE_NAME)
		return
	}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: kind, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit command: %v", err)
	}

	if !resp.Success {
		if kind == daemon.Open {
			log.Errorf("Tunnel %v could not be opened: %v", name, resp.Error)
		} else if kind == daemon.Close {
			log.Errorf("Tunnel %v could not be closed: %v", name, resp.Error)
		} else {
			log.Errorf("Command %v could not be executed for tunnel %v: %v",
				kind, name, resp.Error)
		}
	} else {
		if kind == daemon.Open {
			log.Infof("Opened tunnel %s: %s %v %s via %s",
				log.ColorGreen+t.Name+log.ColorReset,
				t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
		} else if kind == daemon.Close {
			log.Infof("Closed tunnel %s", log.ColorGreen+t.Name+log.ColorReset)
		} else {
			log.Infof("Executed command %v for tunnel %s", kind, t.Name)
		}
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

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  boring list,l                        List tunnels")
	fmt.Println("  boring open,o <name1> [<name2> ...]  Open specified tunnel(s)")
	fmt.Println("  boring close,c <name1> [<name2> ...] Close specified tunnel(s)")
}

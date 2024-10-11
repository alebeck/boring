package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"

	"github.com/creack/pty"
	"golang.org/x/term"
)

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
	case "connect":
		if len(os.Args) < 3 {
			log.Fatalf("'connect' requires a 'name' argument.")
		}
		connectTunnel(os.Args[2])
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
		// Check if config file exists, otherwise we can create it
		if _, statErr := os.Stat(config.FileName); statErr != nil {
			var f *os.File
			if f, err = os.Create(config.FileName); err != nil {
				errs <- fmt.Errorf("could not create config file: %v", err)
				return
			}
			f.Close()
			log.Infof("Created boring config file: %s", config.FileName)
		}
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

	for range 2 {
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
			if kind == daemon.Open {
				openTunnel(name, conf)
			} else if kind == daemon.Close {
				closeTunnel(name, conf)
			} else {
				log.Errorf("Unknown command: %v", kind)
			}
			done <- true
		}()
	}

	for _ = range names {
		<-done
	}
}

func openTunnel(name string, conf *config.Config) {
	t, ok := conf.TunnelsMap[name]
	if !ok {
		log.Errorf("Tunnel '%s' not found in configuration (%s).",
			name, config.FileName)
		return
	}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Open, Tunnel: *t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'open' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel %v could not be opened: %v", name, resp.Error)
	} else {
		log.Infof("Opened tunnel %s: %s %v %s via %s",
			log.ColorGreen+t.Name+log.ColorReset,
			t.LocalAddress, t.Mode, t.RemoteAddress, t.Host)
	}
}

func closeTunnel(name string, conf *config.Config) {
	// The daemon only needs the name for closing. In cases where the
	// config has changed, the name is all we have about the tunnel anyway.
	t := tunnel.Tunnel{Name: name}

	var resp daemon.Resp
	cmd := daemon.Cmd{Kind: daemon.Close, Tunnel: t}
	if err := transmitCmd(cmd, &resp); err != nil {
		log.Errorf("Could not transmit 'close' command: %v", err)
	}

	if !resp.Success {
		log.Errorf("Tunnel %v could not be closed: %v", name, resp.Error)
	} else {
		log.Infof("Closed tunnel %s", log.ColorGreen+t.Name+log.ColorReset)
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

func connectTunnel(name string) {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
	}

	t, ok := conf.TunnelsMap[name]
	if !ok {
		log.Fatalf("Tunnel '%s' not found in configuration (%s).", name, config.FileName)
	}

	// Extract hostname and port from the tunnel configuration
	hostname := t.Host
	port := "22" // Default SSH port
	if strings.Contains(t.RemoteAddress, ":") {
		port = strings.Split(t.RemoteAddress, ":")[1]
	}

	// Check if User is available in the tunnel configuration
	user := "root" // Default user if not specified
	if t.User != "" {
		user = t.User
	}

	// Construct the SSH command
	sshArgs := []string{fmt.Sprintf("%s@%s", user, hostname), "-p", port}
	cmd := exec.Command("ssh", sshArgs...)

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("Failed to start ssh command: %v", err)
	}
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle pty size
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Errorf("Error resizing pty: %v", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Failed to set raw terminal: %v", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// Copy input/output
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, ptmx)
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
	fmt.Println("  boring connect <name>                Connect to specified tunnel via SSH")
}

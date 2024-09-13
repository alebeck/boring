package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "open", "o":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'open' requires at least one 'name' argument.")
			os.Exit(1)
		}
		controlTunnels(os.Args[2:], daemon.Open)
	case "close", "c":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'close' requires at least one 'name' argument.")
			os.Exit(1)
		}
		controlTunnels(os.Args[2:], daemon.Close)
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

func controlTunnels(names []string, kind daemon.CommandKind) {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Remove potential duplicates from names list
	names = slices.Compact(slices.Sorted(slices.Values(names)))

	done := make(chan bool, len(names))

	// Issue concurrent start commands for all tunnels
	for _, name := range names {
		go func() {
			defer func() { done <- true }()

			tunnel, ok := conf.TunnelsMap[name]
			if !ok {
				log.Errorf("Tunnel '%s' not found in configuration (%s).",
					name, config.CONFIG_FILE_NAME)
				return
			}
			cmd := daemon.Command{Kind: kind, Tunnel: *tunnel}

			response, err := transmitCommand(cmd)
			if err != nil {
				log.Errorf("Could not transmit command: %v", err)
			}

			if !response.Success {
				if kind == daemon.Open {
					log.Errorf("Tunnel %v could not be opened: %v", name, response.Error)
				} else if kind == daemon.Close {
					log.Errorf("Tunnel %v could not be closed: %v", name, response.Error)
				} else {
					log.Errorf("Command %v could not be executed for tunnel %v: %v",
						kind, name, response.Error)
				}
			} else {
				if kind == daemon.Open {
					log.Infof("Opened tunnel %s: %s -> %s via %s",
						log.ColorGreen+tunnel.Name+log.ColorReset,
						tunnel.LocalAddress, tunnel.RemoteAddress, tunnel.SSHServer)
				} else if kind == daemon.Close {
					log.Infof("Closed tunnel %s", log.ColorGreen+tunnel.Name+log.ColorReset)
				} else {
					log.Infof("Executed command %v for tunnel %s", kind, tunnel.Name)
				}
			}
		}()
	}

	for _ = range names {
		<-done
	}
}

/*func listTunnels() {
	conf, err := prepare()
	if err != nil {
		log.Fatalf(err.Error())
	}

	cmd := daemon.Command{Kind: daemon.List, Tunnel: nil}
}*/

func transmitCommand(cmd daemon.Command) (daemon.Response, error) {
	empty := daemon.Response{}
	conn, err := daemon.Connect()
	if err != nil {
		return empty, fmt.Errorf("could not connect to daemon: %v", err)
	}
	defer conn.Close()

	if err := ipc.Send(cmd, conn); err != nil {
		return empty, fmt.Errorf("could not send command: %v", err)
	}

	var response daemon.Response
	if err = ipc.Receive(&response, conn); err != nil {
		return empty, fmt.Errorf("could not receive response: %v", err)
	}

	return response, nil
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  boring list,l						List tunnels")
	fmt.Println("  boring open,o <name1> [<name2> ...]  Open specified tunnel(s)")
	fmt.Println("  boring close,c <name1> [<name2> ...] Close specified tunnel(s)")
}

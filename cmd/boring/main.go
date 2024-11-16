package main

import (
	"fmt"
	"os"

	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/log"
)

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
			log.Fatalf("'open' requires at least one 'pattern' argument," +
				" or an '--all/-a' flag.")
		}
		controlTunnels(os.Args[2:], daemon.Open)
	case "close", "c":
		if len(os.Args) < 3 {
			log.Fatalf("'close' requires at least one 'pattern' argument," +
				" or an '--all/-a' flag.")
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
	fmt.Println("  boring list, l                List all tunnels")
	fmt.Println(`  boring open, o (-a | <patterns>...)
    <patterns>...               Open tunnels matching any glob pattern
    -a, --all                   Open all tunnels`)
	fmt.Println("  boring close, c               Close tunnels (same options as 'open')")
	fmt.Println("  boring edit, e                Edit the configuration file")
}

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
			log.Fatalf("'open' requires at least one 'name' argument," +
				" or an '--all/-a' or '--glob/-g' flag.")
		}
		controlTunnels(os.Args[2:], daemon.Open)
	case "close", "c":
		if len(os.Args) < 3 {
			log.Fatalf("'close' requires at least one 'name' argument," +
				" or an '--all/-a' or '--glob/-g' flag.")
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
	fmt.Println("  boring l,list             List tunnels")

	fmt.Println(`  boring o, open (-a | -g <pat> | (<name1> [<name2> ...]))
    -a, --all                 Open all tunnels
    -g, --glob <pat>          Open tunnels matching glob pattern <pat>
    <name1> [<name2> ...]     Open tunnel(s) by name(s)`)

	fmt.Println("  boring c, close           Close tunnels, same options as 'open'")
	fmt.Println("  boring e, edit            Edit configuration file")
}

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/log"
	"golang.org/x/term"
)

var isTerm = term.IsTerminal(int(os.Stdout.Fd()))

var version, commit string

func main() {
	// Run in daemon mode?
	if len(os.Args) == 2 && os.Args[1] == daemon.Flag {
		daemon.Run()
		os.Exit(0)
	}

	initLogging()

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
		log.Printf("Unknown command: %v\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func initLogging() {
	// Use stdout for outputs, indicate if it's an interactive session.
	// We don't use colors under Windows for now.
	useColors := isTerm && runtime.GOOS != "windows"
	log.Init(os.Stdout, isTerm, useColors)
}

func printUsage() {
	v := version
	if v == "" {
		v = "snapshot"
		if commit != "" {
			v += fmt.Sprintf(" (#%s)", commit)
		}
	}

	log.Printf("boring %s\n", v)
	log.Printf("Usage:\n")
	log.Printf("  boring list, l                List all tunnels\n")
	log.Printf(`  boring open, o (-a | <patterns>...)
    <patterns>...               Open tunnels matching any glob pattern
    -a, --all                   Open all tunnels` + "\n")
	log.Printf("  boring close, c               Close tunnels (same options as 'open')\n")
	log.Printf("  boring edit, e                Edit the configuration file\n")
}

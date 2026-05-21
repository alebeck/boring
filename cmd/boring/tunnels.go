package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alebeck/boring/internal/auth"
	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/table"
	"github.com/alebeck/boring/internal/tunnel"
	"golang.org/x/sync/errgroup"
)

const daemonTimeout = 10 * time.Second

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
		if err := ensureDaemon(ctx); err != nil {
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return conf, nil
}

// flow is very linear and I don't mind it at the moment
//
//gocyclo:ignore
func controlTunnels(args []string, kind daemon.CmdKind) {
	var groupFilter string

	if args[0] == "--all" || args[0] == "-a" {
		if len(args) != 1 {
			log.Fatalf("'--all' does not take any additional arguments.")
		}
		args = []string{"*"}
	} else if args[0] == "-g" || args[0] == "--group" {
		if len(args) != 2 {
			log.Fatalf("'-g/--group' requires exactly one group name argument.")
		}
		groupFilter = args[1]
	}

	conf, err := prepare()
	if err != nil {
		log.Fatalf("Startup: %s", err.Error())
	}

	// Get available tunnels for requested command
	ts := conf.TunnelsMap
	if kind == daemon.Close {
		ts, err = getRunningTunnels()
		if err != nil {
			log.Fatalf("Could not get running tunnels: %v", err)
		}
	}

	var m string
	if kind == daemon.Close {
		m = "running "
	}

	var keep map[string]bool

	if groupFilter != "" {
		filterValue := groupFilter
		if groupFilter == "default" {
			filterValue = ""
		}
		keep = filterByGroup(ts, filterValue)
		if len(keep) == 0 {
			log.Fatalf("No %stunnels in group '%s'.", m, groupFilter)
		}
	} else {
		var notMatched []string
		keep, notMatched = filterByPatterns(ts, args)

		if len(keep) == 0 {
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
	}

	// Issue concurrent commands for all tunnels
	var prompter auth.Prompter
	if kind == daemon.Open {
		prompter = openPrompter()
	}
	var g errgroup.Group
	for n := range keep {
		g.Go(func() error {
			if kind == daemon.Open {
				return openTunnel(ts[n], prompter)
			} else if kind == daemon.Close {
				return closeTunnel(ts[n])
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

// syncPrompter serializes Prompt calls so the concurrent open goroutines in
// controlTunnels do not race on the shared terminal — one tunnel's 2FA read
// stealing another's keystrokes, or interleaved prompt text.
type syncPrompter struct {
	inner auth.Prompter
	mu    sync.Mutex
}

func (p *syncPrompter) Prompt(name, instruction string,
	questions []string, echo []bool) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inner.Prompt(name, instruction, questions, echo)
}

// scriptedPrompter answers every challenge question with the given value. It
// is used only by end-to-end tests, which set BORING_AUTH_ANSWERS so that
// `boring open` can authenticate without a terminal.
func scriptedPrompter(answer string) auth.Prompter {
	return auth.FuncPrompter(func(_, _ string, questions []string, _ []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = answer
		}
		return answers, nil
	})
}

// openPrompter returns the auth prompter for `boring open`: a terminal prompter
// for interactive sessions, or one that fails fast when there is no terminal to
// prompt on. The result is wrapped so concurrent open goroutines serialize.
func openPrompter() auth.Prompter {
	// BORING_AUTH_ANSWERS is a documented test-only hook: when set, it
	// supplies a fixed answer to every challenge non-interactively. The env
	// var is unset in normal use, so production behavior is unchanged.
	if answer := os.Getenv("BORING_AUTH_ANSWERS"); answer != "" {
		return scriptedPrompter(answer)
	}
	var base auth.Prompter
	if isTerm {
		base = auth.NewTerminalPrompter()
	} else {
		base = auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
			return nil, errors.New("interactive authentication required but stdin is not a terminal")
		})
	}
	return &syncPrompter{inner: base}
}

func openTunnel(t *tunnel.Desc, prompter auth.Prompter) error {
	resp, err := sendOpen(daemon.Cmd{Kind: daemon.Open, Tunnel: *t}, prompter)
	if err != nil {
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

	log.Infof("Opened tunnel '%s': %s via %s.", log.Green+log.Bold+t.Name+log.Reset,
		describeForwards(t), t.Host)
	return nil
}

// describeForwards renders a tunnel's forwards for the 'open' confirmation
// message. It reads Desc.Forwards (the multi-forward model) rather than the
// legacy singular local/remote/mode fields, which are unset for tunnels
// configured with [[tunnels.forward]] blocks. config.Load guarantees Forwards
// has at least one entry; an empty slice falls back to a neutral label.
func describeForwards(t *tunnel.Desc) string {
	if len(t.Forwards) == 0 {
		return "no forwards"
	}
	parts := make([]string, 0, len(t.Forwards))
	for _, f := range t.Forwards {
		parts = append(parts, fmt.Sprintf("%s %v %s",
			f.DisplayLocal(), f.Mode, f.DisplayRemote()))
	}
	return strings.Join(parts, ", ")
}

func closeTunnel(t *tunnel.Desc) error {
	// Daemon only needs the name, so simplify
	t = &tunnel.Desc{Name: t.Name}
	resp, err := sendCmd(daemon.Cmd{Kind: daemon.Close, Tunnel: *t})
	if err != nil {
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
	resp, err := sendCmd(daemon.Cmd{Kind: daemon.List})
	if err != nil {
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

func listTunnels(args []string) {
	var groupFilter string
	if len(args) > 0 && (args[0] == "-g" || args[0] == "--group") {
		if len(args) != 2 {
			log.Fatalf("'-g/--group' requires exactly one group name argument.")
		}
		groupFilter = args[1]
	} else if len(args) > 0 {
		log.Fatalf("Unknown arguments for 'list'. Use '-g <group>' to filter by group.")
	}

	conf, err := prepare()
	if err != nil {
		log.Fatalf("Startup: %s", err.Error())
	}

	ts, err := getRunningTunnels()
	if err != nil {
		log.Fatalf("Could not list tunnels: %v", err)
	}

	if len(ts) == 0 && len(conf.Tunnels) == 0 {
		log.Infof("No tunnels configured.")
		return
	}

	all := tunnel.Order(conf.Tunnels, ts)

	// Filter by group if requested
	if groupFilter != "" {
		filterValue := groupFilter
		if groupFilter == "default" {
			filterValue = ""
		}
		var filtered []*tunnel.Desc
		for _, t := range all {
			if t.Group == filterValue {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("No tunnels in group '%s'.", groupFilter)
		}
		all = filtered
	}

	printTunnelList(all)
}

func printTunnelList(all []*tunnel.Desc) {
	// If any tunnel has a non-empty group, use grouped display
	hasGroups := false
	for _, t := range all {
		if t.Group != "" {
			hasGroups = true
			break
		}
	}
	if !hasGroups {
		log.Emitf("%v", tunnelTable(all))
		return
	}

	// Grouped display
	groups := make(map[string][]*tunnel.Desc)
	var groupKeys []string
	for _, t := range all {
		if _, ok := groups[t.Group]; !ok && t.Group != "" {
			groupKeys = append(groupKeys, t.Group)
		}
		groups[t.Group] = append(groups[t.Group], t)
	}

	// Default group first, if present
	if _, ok := groups[""]; ok {
		groupKeys = append([]string{""}, groupKeys...)
	}

	for i, gk := range groupKeys {
		if i > 0 {
			log.Emitf("\n")
		}
		header := gk
		if header == "" {
			header = "default"
		}
		log.Emitf("%s[%s]%s\n", log.Bold+log.Blue, header, log.Reset)
		log.Emitf("%v", tunnelTable(groups[gk]))
	}
}

// tunnelTable builds the grouped-tree listing for `boring list`. A
// single-forward tunnel renders inline on one line; a multi-forward tunnel
// renders a connection-level header row plus one indented sub-row per forward.
// It reads Desc.Forwards (the multi-forward model), not the legacy singular
// local/remote/mode fields.
func tunnelTable(tunnels []*tunnel.Desc) *table.TunnelTable {
	tbl := table.NewTunnelTable()
	for _, t := range tunnels {
		tbl.Add(table.TunnelRow{
			Status:   status(t),
			Name:     t.Name,
			Via:      t.Host,
			Forwards: forwardRows(t),
		})
	}
	return tbl
}

// forwardRows converts a tunnel's forwards into renderable sub-rows. config.Load
// guarantees Forwards has at least one entry; an empty slice yields no rows so
// the tunnel still renders its header line.
func forwardRows(t *tunnel.Desc) []table.ForwardRow {
	rows := make([]table.ForwardRow, 0, len(t.Forwards))
	for _, f := range t.Forwards {
		rows = append(rows, table.ForwardRow{
			Label:  f.Label(),
			Local:  f.DisplayLocal(),
			Mode:   f.Mode.String(),
			Remote: f.DisplayRemote(),
		})
	}
	return rows
}

func filterByPatterns(ts map[string]*tunnel.Desc, pats []string) (map[string]bool, []string) {
	keep := make(map[string]bool, len(ts))
	var notMatched []string
	for _, pat := range pats {
		n, err := filterGlob(ts, keep, pat)
		if err != nil {
			log.Fatalf("Malformed glob pattern '%v'.", pat)
		}
		if n == 0 {
			notMatched = append(notMatched, pat)
		}
	}
	return keep, notMatched
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

func filterByGroup(ts map[string]*tunnel.Desc, group string) map[string]bool {
	keep := make(map[string]bool)
	for name, t := range ts {
		if t.Group == group {
			keep[name] = true
		}
	}
	return keep
}

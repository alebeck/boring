package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

// testTunnels verifies the SSH handshake and authentication for every tunnel
// matching the given patterns. It runs entirely in the CLI process: no daemon
// is spawned and no listener is opened. Tunnels are tested sequentially in
// config order for deterministic output. The process exits with status 1 if
// any tunnel fails to connect.
func testTunnels(args []string) {
	conf, err := config.Load()
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	keep, notMatched := filterByPatterns(conf.TunnelsMap, args)
	if len(keep) == 0 {
		msg := fmt.Sprintf("No tunnels match pattern '%s'.", args[0])
		if len(args) > 1 {
			msg = "No tunnels match any provided pattern."
		}
		log.Fatalf("%s", msg)
	}

	// Warn about patterns that matched nothing, mirroring controlTunnels.
	for _, pat := range notMatched {
		log.Warningf("No tunnels match pattern '%s'.", pat)
	}

	// Build the prompter once and reuse it so 2FA / passphrase tunnels can be
	// tested. openPrompter honors the BORING_AUTH_ANSWERS test hook.
	prompter := openPrompter()

	failed := false
	for i := range conf.Tunnels {
		t := &conf.Tunnels[i]
		if !keep[t.Name] {
			continue
		}
		log.Infof("Testing %s...", t.Name)
		res := tunnel.TestConnection(t, prompter)
		if res.OK {
			log.Infof("%s: connection OK (%v)", t.Name,
				res.Duration.Round(time.Millisecond))
			continue
		}
		log.Errorf("%s: connection failed: %s", t.Name, res.Err)
		failed = true
	}

	if failed {
		os.Exit(1)
	}
}

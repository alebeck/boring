package ssh_config

import (
	"crypto/ed25519"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh/agent"
)

// startTestAgent serves an in-memory SSH agent holding the given keys on a
// fresh unix socket and returns its path.
func startTestAgent(t *testing.T, keys ...ed25519.PrivateKey) string {
	t.Helper()
	keyring := agent.NewKeyring()
	for _, k := range keys {
		if err := keyring.Add(agent.AddedKey{PrivateKey: k}); err != nil {
			t.Fatalf("agent add: %v", err)
		}
	}
	// A short temp dir: macOS caps unix socket paths at ~104 bytes, and
	// t.TempDir() embeds the (long) test name.
	dir, err := os.MkdirTemp("", "ag")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	return sock
}

// TestMakeSignersConfiguredKeyBeforeAgentKeys locks the fix for the bug where
// an explicitly configured IdentityFile key was offered AFTER every
// unconfigured agent key. On a host reached through an agent holding many
// keys, the configured key was never tried before the server's MaxAuthTries
// limit, causing "Too many authentication failures".
func TestMakeSignersConfiguredKeyBeforeAgentKeys(t *testing.T) {
	// Two keys the agent holds but which are not configured for the host.
	_, a1, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, a2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sock := startTestAgent(t, a1, a2)

	sc := &SSHConfig{
		IdentityFiles: []string{plainKeyPath}, // the explicitly configured key
		AgentSock:     sock,
	}
	sigs, err := sc.makeSigners()
	if err != nil {
		t.Fatalf("makeSigners: %v", err)
	}
	if len(sigs) != 3 {
		t.Fatalf("got %d signers, want 3 (1 configured file key + 2 agent keys)", len(sigs))
	}

	fileSigner, err := loadPrivateKeyInteractive(plainKeyPath, nil)
	if err != nil {
		t.Fatalf("load configured key: %v", err)
	}
	if keyFP(sigs[0].PublicKey()) != keyFP(fileSigner.PublicKey()) {
		t.Fatal("the configured IdentityFile key must be offered first, " +
			"before unconfigured agent keys")
	}
}

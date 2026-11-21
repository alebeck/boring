package ssh_config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func writeKeyPair(t *testing.T, dir, name string) (privPath, pubPath string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	privPath = filepath.Join(dir, name)
	if err := os.WriteFile(privPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	pubPath = privPath + ".pub"
	if err := os.WriteFile(pubPath, ssh.MarshalAuthorizedKey(sshPub), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestLoadIdentityPrivateKey(t *testing.T) {
	priv, _ := writeKeyPair(t, t.TempDir(), "id_test")

	s, fp, ok := loadIdentity(priv)
	if !ok {
		t.Fatal("expected loadIdentity to succeed")
	}
	if s == nil {
		t.Fatal("expected non-nil signer for private key path")
	}
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}

// OpenSSH allows `IdentityFile foo.pub` when the private key is held by the agent
// or a hardware token etc. loadIdentity must succeed and return the fingerprint
// so that the agent's matching key isn't filtered out under IdentitiesOnly.
func TestLoadIdentityPublicKeyOnly(t *testing.T) {
	dir := t.TempDir()
	priv, pub := writeKeyPair(t, dir, "id_test")

	// Remove the private key
	if err := os.Remove(priv); err != nil {
		t.Fatal(err)
	}

	s, fp, ok := loadIdentity(pub)
	if !ok {
		t.Fatal("expected loadIdentity to succeed for .pub-only IdentityFile")
	}
	if s != nil {
		t.Fatal("expected nil signer when only public key is available")
	}
	if fp == "" {
		t.Fatal("expected non-empty fingerprint from .pub")
	}
}

// Public-key fingerprint resolved from a .pub-only IdentityFile must match the
// fingerprint the private key would have produced
func TestLoadIdentityFingerprintsMatch(t *testing.T) {
	priv, pub := writeKeyPair(t, t.TempDir(), "id_test")

	_, fpPriv, ok := loadIdentity(priv)
	if !ok {
		t.Fatal("private-key load failed")
	}
	_, fpPub, ok := loadIdentity(pub)
	if !ok {
		t.Fatal("public-key load failed")
	}
	if fpPriv != fpPub {
		t.Fatalf("fingerprint mismatch: priv=%x pub=%x", fpPriv, fpPub)
	}
}

// If f isn't a usable private key but a sibling f+".pub" exists, the sibling
// fingerprint should still be picked up.
func TestLoadIdentitySiblingPublicKey(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "id_test")
	if err := os.WriteFile(priv, []byte("not a private key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, realPub := writeKeyPair(t, dir, "id_other")
	if err := os.Rename(realPub, priv+".pub"); err != nil {
		t.Fatal(err)
	}

	s, fp, ok := loadIdentity(priv)
	if !ok {
		t.Fatal("expected sibling .pub to be loaded")
	}
	if s != nil {
		t.Fatal("expected nil signer when only sibling pub exists")
	}
	if fp == "" {
		t.Fatal("expected non-empty fingerprint from sibling")
	}
}

func TestLoadIdentityMissing(t *testing.T) {
	s, fp, ok := loadIdentity(filepath.Join(t.TempDir(), "does-not-exist"))
	if ok || s != nil || fp != "" {
		t.Fatalf("expected failure, got s=%v fp=%q ok=%v", s, fp, ok)
	}
}

// When no Port is specified in the SSH config, the default of 22 must be used.
func TestParseSSHConfigDefaultPort(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("Host myhost\n\tHostName example.com\n\tUser bob\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	old := overrideConfig
	overrideConfig = cfg
	t.Cleanup(func() { overrideConfig = old })

	sc, err := ParseSSHConfig("myhost", "bob", 0)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Port != 22 {
		t.Errorf("Port = %d, want 22", sc.Port)
	}
}

// Tokens and environment variables in values must be expanded like ssh(1) does
func TestParseSSHConfigTokens(t *testing.T) {
	local, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_DIR", "/env")
	tests := []struct {
		config, user string
		port         int
		got          func(*SSHConfig) string
		want         string
	}{
		{"HostName example.com\nProxyJump %h\n", "", 0, func(sc *SSHConfig) string { return sc.Jumps[0].host }, "example.com"},
		{"User bob\nCertificateFile %r-cert.pub\n", "", 0, func(sc *SSHConfig) string { return sc.CertificateFiles[0] }, "bob-cert.pub"},
		{"IdentityFile %%h\n", "", 0, func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "%h"},
		{"User bob\nIdentityFile %r\n", "alice", 0, func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "alice"},
		{"IdentityFile %r\n", "", 0, func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, local.Username},
		{"Port 2222\nIdentityFile %p\n", "", 3333, func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "3333"},
		{"IdentityFile ${TEST_DIR}/%r\n", "bob", 0, func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "/env/bob"},
		{"CertificateFile ${TEST_DIR}/cert\n", "", 0, func(sc *SSHConfig) string { return sc.CertificateFiles[0] }, "/env/cert"},
		{"IdentityAgent ${TEST_DIR}/agent.sock\n", "", 0, func(sc *SSHConfig) string { return sc.IdentityAgent }, "/env/agent.sock"},
		{"UserKnownHostsFile ${TEST_DIR}/known_hosts\n", "", 0, func(sc *SSHConfig) string { return sc.KnownHostsFiles[len(sc.KnownHostsFiles)-1] }, "/env/known_hosts"},
	}

	old := overrideConfig
	t.Cleanup(func() { overrideConfig = old })
	parse := func(config, user string, port int) (*SSHConfig, error) {
		overrideConfig = filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(overrideConfig, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		return ParseSSHConfig("alias", user, port)
	}
	for _, tt := range tests {
		sc, err := parse(tt.config, tt.user, tt.port)
		if err != nil {
			t.Fatal(err)
		}
		if got := tt.got(sc); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.config, got, tt.want)
		}
	}

	// Like ssh(1), unset environment variables are an error
	if _, err := parse("IdentityFile ${TEST_UNSET}/id\n", "", 0); err == nil {
		t.Error("expected error for unset environment variable")
	}
}

func TestAgentSock(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("USERPROFILE", "/home/u") // home directory on Windows
	t.Setenv("SSH_AUTH_SOCK", "/tmp/default.sock")
	t.Setenv("MY_SOCK", "/tmp/my.sock")

	for in, want := range map[string]string{
		"":                    "/tmp/default.sock",
		"SSH_AUTH_SOCK":       "/tmp/default.sock",
		"$MY_SOCK":            "/tmp/my.sock",
		"~/agent.sock":        filepath.Join("/home/u", "agent.sock"),
		"/path/to/agent.sock": "/path/to/agent.sock",
	} {
		if got := agentSock(in); got != want {
			t.Errorf("agentSock(%q) = %q, want %q", in, got, want)
		}
	}
}

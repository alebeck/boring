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

	sc, err := ParseSSHConfig("myhost", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Port != 22 {
		t.Errorf("Port = %d, want 22", sc.Port)
	}
}

// Tokens in values must be expanded like ssh(1) does
func TestParseSSHConfigTokens(t *testing.T) {
	local, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		config, user string
		got          func(*SSHConfig) string
		want         string
	}{
		{"HostName example.com\nProxyJump %h\n", "", func(sc *SSHConfig) string { return sc.Jumps[0].host }, "example.com"},
		{"User bob\nCertificateFile %r-cert.pub\n", "", func(sc *SSHConfig) string { return sc.CertificateFiles[0] }, "bob-cert.pub"},
		{"IdentityFile %%h\n", "", func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "%h"},
		{"User bob\nIdentityFile %r\n", "alice", func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, "alice"},
		{"IdentityFile %r\n", "", func(sc *SSHConfig) string { return sc.IdentityFiles[0] }, local.Username},
	}

	old := overrideConfig
	t.Cleanup(func() { overrideConfig = old })
	for _, tt := range tests {
		overrideConfig = filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(overrideConfig, []byte(tt.config), 0o600); err != nil {
			t.Fatal(err)
		}
		sc, err := ParseSSHConfig("alias", tt.user)
		if err != nil {
			t.Fatal(err)
		}
		if got := tt.got(sc); got != tt.want {
			t.Errorf("%q: got %q, want %q", tt.config, got, tt.want)
		}
	}
}

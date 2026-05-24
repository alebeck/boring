package ssh_config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
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

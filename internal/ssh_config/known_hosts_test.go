package ssh_config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const testHostPort = "127.0.0.1:2222"

func edPub(t *testing.T) ssh.PublicKey {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func rsaPub(t *testing.T) ssh.PublicKey {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// callbackFor writes lines to a temp known_hosts file and returns the real
// knownhosts.New callback for it.
func callbackFor(t *testing.T, lines string) ssh.HostKeyCallback {
	p := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(p, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	cb, err := knownhosts.New(p)
	if err != nil {
		t.Fatal(err)
	}
	return cb
}

func TestExtractHostKeyAlgosCertAuthority(t *testing.T) {
	ca := edPub(t)
	cb := callbackFor(t,
		"@cert-authority "+knownhosts.Line([]string{testHostPort}, ca)+"\n")

	// A CA line carries no host-key-type info, so all cert algorithms are
	// offered regardless of the CA key's own type (here ed25519).
	algos := extractHostKeyAlgos(cb, testHostPort)
	if !reflect.DeepEqual(algos, allCertAlgos) {
		t.Fatalf("got %v, want %v", algos, allCertAlgos)
	}
}

// Plain (non-CA) known_hosts entries must yield only plain algorithms,
// including the RSA SHA-2 expansions, and never a *-cert-v01 algorithm.
func TestExtractHostKeyAlgosPlain(t *testing.T) {
	rsaKey := rsaPub(t)
	edKey := edPub(t)
	cb := callbackFor(t,
		knownhosts.Line([]string{testHostPort}, rsaKey)+"\n"+
			knownhosts.Line([]string{testHostPort}, edKey)+"\n")

	algos := extractHostKeyAlgos(cb, testHostPort)
	want := []string{
		ssh.KeyAlgoRSA,
		ssh.KeyAlgoRSASHA256,
		ssh.KeyAlgoRSASHA512,
		ssh.KeyAlgoED25519,
	}
	if !reflect.DeepEqual(algos, want) {
		t.Fatalf("got %v, want %v", algos, want)
	}
}

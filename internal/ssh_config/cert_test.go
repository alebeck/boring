package ssh_config

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func newTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAsCert(t *testing.T) {
	s := newTestSigner(t)
	c := &ssh.Certificate{
		Key:         s.PublicKey(),
		CertType:    ssh.UserCert,
		ValidBefore: ssh.CertTimeInfinity,
	}
	if err := c.SignCert(rand.Reader, newTestSigner(t)); err != nil {
		t.Fatal(err)
	}

	if got, ok := asCert(c); !ok || string(got.Marshal()) != string(c.Marshal()) {
		t.Error("certificate not recognized")
	}
	// What the agent client returns for a certificate identity
	wrapped := &agent.Key{Format: c.Type(), Blob: c.Marshal()}
	if got, ok := asCert(wrapped); !ok || string(got.Marshal()) != string(c.Marshal()) {
		t.Error("certificate from agent not recognized")
	}
	if _, ok := asCert(s.PublicKey()); ok {
		t.Error("plain key taken for a certificate")
	}
	if _, ok := asCert(dummyKey{}); ok {
		t.Error("unparsable key taken for a certificate")
	}
}

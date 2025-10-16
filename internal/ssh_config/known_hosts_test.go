package ssh_config

import (
	"net"
	"reflect"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type rsaTestKey struct{}

func (k *rsaTestKey) Type() string {
	return ssh.KeyAlgoRSA
}

func (k *rsaTestKey) Marshal() []byte {
	return []byte{}
}

func (k *rsaTestKey) Verify(_ []byte, _ *ssh.Signature) error {
	return nil
}

type edTestKey struct{}

func (k *edTestKey) Type() string {
	return ssh.KeyAlgoED25519
}

func (k *edTestKey) Marshal() []byte {
	return []byte{}
}

func (k *edTestKey) Verify(_ []byte, _ *ssh.Signature) error {
	return nil
}

// Tests that extractHostKeyAlgos correctly extracts algorithms from a
// HostKeyCallback that returns a KeyError with known keys.
func TestExtractHostKeyAlgos(t *testing.T) {
	cb := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return &knownhosts.KeyError{
			Want: []knownhosts.KnownKey{
				{Key: &edTestKey{}},
				{Key: &rsaTestKey{}},
				{Key: nil}, // should be ignored
			},
		}
	}
	algos := extractHostKeyAlgos(cb, "example.com")
	if !reflect.DeepEqual(algos, []string{
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoRSA,
		ssh.KeyAlgoRSASHA256,
		ssh.KeyAlgoRSASHA512,
	}) {
		t.Errorf("unexpected algorithms: %v", algos)
	}
}

package ssh_config

import (
	"errors"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type dummyKey struct{}

func (dummyKey) Type() string                        { return "dummy" }
func (dummyKey) Marshal() []byte                     { return nil }
func (dummyKey) Verify([]byte, *ssh.Signature) error { return errors.New("dummy key") }

// Follows the idea from https://github.com/golang/go/issues/29286#issuecomment-1160958614
// to extract available keys from the known_hosts.
func extractHostKeyAlgos(cb ssh.HostKeyCallback, host string) (res []string) {
	addr := &net.TCPAddr{IP: net.IPv4zero}
	key := dummyKey{}
	var ke *knownhosts.KeyError

	if err := cb(host, addr, key); errors.As(err, &ke) {
		for _, k := range ke.Want {
			if k.Key == nil {
				continue
			}
			res = append(res, k.Key.Type())
			if k.Key.Type() == ssh.KeyAlgoRSA {
				res = append(res, ssh.KeyAlgoRSASHA256)
				res = append(res, ssh.KeyAlgoRSASHA512)
			}
		}
	}
	return
}

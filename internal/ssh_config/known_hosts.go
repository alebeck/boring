package ssh_config

import (
	"errors"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type dummyKey struct{}

func (dummyKey) Type() string                        { return "dummy" }
func (dummyKey) Marshal() []byte                     { return nil }
func (dummyKey) Verify([]byte, *ssh.Signature) error { return errors.New("dummy key") }

var allCertAlgos = []string{
	ssh.CertAlgoED25519v01,
	ssh.CertAlgoECDSA256v01,
	ssh.CertAlgoECDSA384v01,
	ssh.CertAlgoECDSA521v01,
	ssh.CertAlgoRSASHA512v01,
	ssh.CertAlgoRSASHA256v01,
	ssh.CertAlgoRSAv01,
}

// isHostAuthority reports whether a key is trusted as a certificate authority
// for host.
func isHostAuthority(cb ssh.HostKeyCallback, host string, addr net.Addr, key ssh.PublicKey) bool {
	cert := &ssh.Certificate{
		Key:          key,
		CertType:     ssh.HostCert,
		SignatureKey: key,
		ValidBefore:  ssh.CertTimeInfinity,
		Signature:    &ssh.Signature{Format: key.Type()},
	}
	err := cb(host, addr, cert)
	return err != nil && !strings.Contains(err.Error(), "no authorities for hostname")
}

// Follows the idea from https://github.com/golang/go/issues/29286#issuecomment-1160958614
// to extract available host key algorithms from the known_hosts. Keys
// belonging to a trusted CA yield certificate algorithms, so the server
// presents its host certificate instead of a plain key.
func extractHostKeyAlgos(cb ssh.HostKeyCallback, host string) (res []string) {
	addr := &net.TCPAddr{IP: net.IPv4zero}
	var ke *knownhosts.KeyError

	if err := cb(host, addr, dummyKey{}); errors.As(err, &ke) {
		certsAdded := false
		for _, k := range ke.Want {
			if k.Key == nil {
				continue
			}
			if isHostAuthority(cb, host, addr, k.Key) {
				if !certsAdded {
					res = append(res, allCertAlgos...)
					certsAdded = true
				}
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

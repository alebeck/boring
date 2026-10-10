package e2e

import (
	"context"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"os"
	"path/filepath"
)

const (
	clientKeyFile = "../testdata/keys/client"
)

func startAgent(sock string) (context.CancelFunc, error) {
	return startAgentWithCert(sock, "")
}

// startAgentWithCert starts an agent holding the client key. If certFile is
// set, the key is added together with that certificate.
func startAgentWithCert(sock, certFile string) (context.CancelFunc, error) {
	// Read and parse the private key
	keyBytes, err := os.ReadFile(clientKeyFile)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	key := agent.AddedKey{
		PrivateKey: signer,
		Comment:    filepath.Base(clientKeyFile),
	}
	if certFile != "" {
		certBytes, err := os.ReadFile(certFile)
		if err != nil {
			return nil, err
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey(certBytes)
		if err != nil {
			return nil, err
		}
		cert, ok := pub.(*ssh.Certificate)
		if !ok {
			return nil, fmt.Errorf("%s is not a certificate", certFile)
		}
		key.Certificate = cert
	}

	// Create agent and add the key
	kr := agent.NewKeyring()
	if err := kr.Add(key); err != nil {
		return nil, err
	}

	// Create a Unix socket and serve the agent.
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}

	// Accept loop
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				agent.ServeAgent(kr, c)
			}()
		}
	}()

	cancel := func() {
		ln.Close()
	}
	return cancel, nil
}

package e2e

import (
	"context"
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
	// Read and parse the private key
	keyBytes, err := os.ReadFile(clientKeyFile)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParseRawPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	// Create agent and add the key
	kr := agent.NewKeyring()
	if err := kr.Add(agent.AddedKey{
		PrivateKey: signer,
		Comment:    filepath.Base(clientKeyFile),
	}); err != nil {
		return nil, err
	}

	// Listen on the socket and serve the agent.
	ln, err := listenAgent(sock)
	if err != nil {
		return nil, err
	}

	// Accept loop
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, c)
			go func() {
				defer c.Close()
				agent.ServeAgent(kr, c)
			}()
		}
	}()

	// Like a stopped agent, close all client connections
	cancel := func() {
		ln.Close()
		<-done
		for _, c := range conns {
			c.Close()
		}
	}
	return cancel, nil
}

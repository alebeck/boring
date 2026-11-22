package agent

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// conn connects to the agent at sock for every request. It must not implement
// io.Closer, as the agent client would then read from it in the background.
type conn struct {
	sock string
	rwc  io.ReadWriteCloser
}

func (c *conn) Write(p []byte) (n int, err error) {
	if c.rwc != nil {
		c.rwc.Close()
	}
	if c.rwc, err = dial(c.sock); err != nil {
		return 0, err
	}
	return c.rwc.Write(p)
}

func (c *conn) Read(p []byte) (int, error) {
	if c.rwc == nil {
		return 0, io.ErrClosedPipe
	}
	return c.rwc.Read(p)
}

var (
	// Keep one agent client per socket for all connection attempts
	clients = map[string]agent.ExtendedAgent{}
	mu      sync.Mutex
)

func getAgent(sock string) (agent.ExtendedAgent, error) {
	if sock == "" {
		sock = defaultSock
	}
	if sock == "" {
		return nil, fmt.Errorf("agent socket not set (SSH_AUTH_SOCK or IdentityAgent)")
	}

	mu.Lock()
	defer mu.Unlock()

	if c, ok := clients[sock]; ok {
		return c, nil
	}

	c := agent.NewClient(&conn{sock: sock})
	clients[sock] = c
	return c, nil
}

func GetSigners(sock string) ([]ssh.Signer, error) {
	agent, err := getAgent(sock)
	if err != nil {
		return nil, err
	}

	signers, err := agent.Signers()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve signers from agent: %v", err)
	}

	return signers, nil
}

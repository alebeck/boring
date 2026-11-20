package agent

import (
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// conn is a connection to the agent at sock, which is (re-)established on
// demand, as agents may close it at any time, e.g. when they get locked.
//
// It must not implement io.Closer, since the agent client would then read
// from it in the background, rather than once per request.
type conn struct {
	sock string
	rwc  io.ReadWriteCloser
}

func (c *conn) Write(p []byte) (n int, err error) {
	// Writing fails if the agent closed the connection, so try a new one then
	if c.rwc != nil {
		if n, err = c.do(c.rwc.Write, p); err == nil {
			return n, nil
		}
	}
	if c.rwc, err = dial(c.sock); err != nil {
		return 0, err
	}
	return c.do(c.rwc.Write, p)
}

func (c *conn) Read(p []byte) (int, error) {
	if c.rwc == nil {
		return 0, io.ErrClosedPipe
	}
	return c.do(c.rwc.Read, p)
}

// do closes the connection if op fails, so the next request uses a new one
func (c *conn) do(op func([]byte) (int, error), p []byte) (int, error) {
	n, err := op(p)
	if err != nil {
		c.rwc.Close()
		c.rwc = nil
	}
	return n, err
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

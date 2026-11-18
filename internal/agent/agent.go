package agent

import (
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

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

	conn, err := dial(sock)
	if err != nil {
		return nil, fmt.Errorf("could not dial agent: %v", err)
	}

	c := agent.NewClient(conn)
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

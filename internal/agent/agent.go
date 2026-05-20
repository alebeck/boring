package agent

import (
	"fmt"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	// insts caches one agent client per socket path, so repeated connection
	// attempts reuse a single connection.
	insts = make(map[string]agent.ExtendedAgent)
	mu    sync.Mutex
)

func getAgent(sock string) (agent.ExtendedAgent, error) {
	if sock == "" {
		return nil, fmt.Errorf("no SSH agent socket configured")
	}

	mu.Lock()
	defer mu.Unlock()

	if a, ok := insts[sock]; ok {
		return a, nil
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("could not dial agent at %s: %w", sock, err)
	}

	a := agent.NewClient(conn)
	insts[sock] = a
	return a, nil
}

// GetSigners returns the signers held by the SSH agent listening on the given
// socket path.
func GetSigners(sock string) ([]ssh.Signer, error) {
	a, err := getAgent(sock)
	if err != nil {
		return nil, err
	}

	signers, err := a.Signers()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve signers from agent: %w", err)
	}

	return signers, nil
}

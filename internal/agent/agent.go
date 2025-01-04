package agent

import (
	"fmt"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	// Keep a single agent instance for all connection attempts
	inst agent.ExtendedAgent
	mu   sync.Mutex
)

func getAgent() (agent.ExtendedAgent, error) {
	mu.Lock()
	defer mu.Unlock()

	if inst != nil {
		return inst, nil
	}

	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK is not set")
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("could not dial agent: %v", err)
	}

	inst = agent.NewClient(conn)
	return inst, nil
}

func GetSigners() ([]ssh.Signer, error) {
	agent, err := getAgent()
	if err != nil {
		return nil, err
	}

	signers, err := agent.Signers()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve signers from agent: %v", err)
	}

	return signers, nil
}

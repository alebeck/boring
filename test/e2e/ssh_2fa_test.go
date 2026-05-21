package e2e

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

// makeChallenge returns a keyboard-interactive challenge function that always
// answers each question with the given code.
func makeChallenge(code string) ssh.KeyboardInteractiveChallenge {
	return func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = code
		}
		return answers, nil
	}
}

// TestSSHServer2FAFixture verifies the keyboard-interactive (2FA) test SSH
// server fixture directly, without involving the boring daemon: the correct
// code is accepted and a wrong code is rejected.
func TestSSHServer2FAFixture(t *testing.T) {
	s, err := startServer2FA()
	if err != nil {
		t.Fatalf("failed to start 2FA SSH server: %v", err)
	}
	t.Cleanup(s.cleanup)

	t.Run("correct code succeeds", func(t *testing.T) {
		cfg := &ssh.ClientConfig{
			User: "test",
			Auth: []ssh.AuthMethod{
				ssh.KeyboardInteractive(makeChallenge(test2FACode)),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         connTimeout,
		}
		client, err := ssh.Dial("tcp", s.addr, cfg)
		if err != nil {
			t.Fatalf("dial with correct 2FA code should succeed: %v", err)
		}
		client.Close()
	})

	t.Run("wrong code fails", func(t *testing.T) {
		cfg := &ssh.ClientConfig{
			User: "test",
			Auth: []ssh.AuthMethod{
				ssh.KeyboardInteractive(makeChallenge("000000")),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         connTimeout,
		}
		client, err := ssh.Dial("tcp", s.addr, cfg)
		if err == nil {
			client.Close()
			t.Fatal("dial with wrong 2FA code should fail")
		}
	})
}

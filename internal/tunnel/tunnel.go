package tunnel

import (
	"fmt"
	"os/exec"

	"github.com/alebeck/boring/internal/log"
)

type Address struct {
	Host string
	Port int
}

// Tunnel represents an SSH tunnel configuration and management
type Tunnel struct {
	Name          string    `toml:"name" json:"name"`
	LocalAddress  string    `toml:"local" json:"local"`
	RemoteAddress string    `toml:"remote" json:"remote"`
	SSHServer     string    `toml:"ssh" json:"ssh"`
	IdentityFile  string    `toml:"identity" json:"identity"`
	Cmd           *exec.Cmd `toml:"-" json:"-"`
	Disconnected  chan bool `toml:"-" json:"-"`
}

// Open establishes the SSH tunnel by running the SSH command in a subprocess
func (t *Tunnel) Open() error {
	args := []string{"-L",
		fmt.Sprintf("%s:%s", t.LocalAddress, t.RemoteAddress),
		fmt.Sprintf("%s", t.SSHServer), "-N"}

	if t.IdentityFile != "" {
		args = append([]string{"-i", t.IdentityFile}, args...)
	}

	t.Cmd = exec.Command("ssh", args...)
	t.Disconnected = make(chan bool)

	if err := t.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to open SSH tunnel: %w", err)
	}

	log.Infof("Opened tunnel %s: %s -> %s via %s",
		log.ColorGreen+t.Name+log.ColorReset,
		t.LocalAddress, t.RemoteAddress, t.SSHServer)

	go t.monitorProcess()

	return nil
}

func (t *Tunnel) Close() error {
	if t.Cmd == nil || t.Cmd.Process == nil {
		return fmt.Errorf("no running SSH tunnel to close")
	}

	if err := t.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to close SSH tunnel: %w", err)
	}

	log.Infof("Closed tunnel %s", log.ColorGreen+t.Name+log.ColorReset)
	return nil
}

func (t *Tunnel) monitorProcess() {
	t.Cmd.Wait()
	t.Disconnected <- true
}

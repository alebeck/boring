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
	Name          string    `toml:"name"`
	LocalAddress  string    `toml:"local"`
	RemoteAddress string    `toml:"remote"`
	SSHServer     string    `toml:"ssh"`
	IdentityFile  string    `toml:"identity"`
	Cmd           *exec.Cmd `toml:"-"`
	Disconnected  chan bool `toml:"-"`
}

// NewTunnel initializes a new Tunnel instance
func NewTunnel(localAddress, remoteAddress, sshServer, identityFile string) *Tunnel {
	return &Tunnel{
		LocalAddress:  localAddress,
		RemoteAddress: remoteAddress,
		SSHServer:     sshServer,
		IdentityFile:  identityFile,
		Disconnected:  make(chan bool),
	}
}

// Start establishes the SSH tunnel by running the SSH command in a subprocess
func (t *Tunnel) Start() error {
	args := []string{"-L",
		fmt.Sprintf("%s:%s", t.LocalAddress, t.RemoteAddress),
		fmt.Sprintf("%s", t.SSHServer), "-N"}

	if t.IdentityFile != "" {
		args = append([]string{"-i", t.IdentityFile}, args...)
	}

	t.Cmd = exec.Command("ssh", args...)

	if err := t.Cmd.Start(); err != nil {
		return fmt.Errorf("failed to start SSH tunnel: %w", err)
	}

	log.Infof("Started tunnel %s: %s -> %s via %s",
		log.ColorGreen+t.Name+log.ColorReset,
		t.LocalAddress, t.RemoteAddress, t.SSHServer)

	go t.monitorProcess()

	return nil
}

func (t *Tunnel) Stop() error {
	if t.Cmd == nil || t.Cmd.Process == nil {
		return fmt.Errorf("no running SSH tunnel to stop")
	}

	if err := t.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to stop SSH tunnel: %w", err)
	}

	log.Infof("Stopped tunnel %s", log.ColorGreen+t.Name+log.ColorReset)
	return nil
}

func (t *Tunnel) monitorProcess() {
	t.Cmd.Wait()
	log.Infof("SSH Tunnel on port %s has disconnected", t.LocalAddress)
	t.Disconnected <- true
}

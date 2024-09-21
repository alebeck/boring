package tunnel

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/alebeck/boring/internal/log"
	"golang.org/x/crypto/ssh"
)

type Status int

const (
	Closed Status = iota
	Open
	Reconnecting
)

const (
	RECONNECT_WAIT    = 2 * time.Millisecond
	RECONNECT_TIMEOUT = 10 * time.Minute
)

// Tunnel represents an SSH tunnel configuration and management
type Tunnel struct {
	Name          string            `toml:"name" json:"name"`
	LocalAddress  string            `toml:"local" json:"local"`
	RemoteAddress string            `toml:"remote" json:"remote"`
	Host          string            `toml:"host" json:"host"`
	User          string            `toml:"user" json:"user"`
	IdentityFile  string            `toml:"identity" json:"identity"`
	Port          int               `toml:"port" json:"port"`
	Status        Status            `toml:"-" json:"status"`
	Closed        chan struct{}     `toml:"-" json:"-"`
	client        *ssh.Client       `toml:"-" json:"-"`
	clientConfig  *ssh.ClientConfig `toml:"-" json:"-"`
	listener      net.Listener      `toml:"-" json:"-"`
	stop          chan struct{}     `toml:"-" json:"-"`
}

func (t *Tunnel) Open() error {
	var err error

	if err := t.parseSSHConf(); err != nil {
		return fmt.Errorf("could not parse ssh config: %v", err)
	}

	if err := t.validate(); err != nil {
		return fmt.Errorf("invalid tunnel: %v", err)
	}

	if t.clientConfig == nil {
		t.clientConfig, err = t.makeClientConf()
		if err != nil {
			return fmt.Errorf("could not make client config: %v", err)
		}
	}

	remoteAddr := fmt.Sprintf("%v:%v", t.Host, t.Port)
	t.client, err = ssh.Dial("tcp", remoteAddr, t.clientConfig)
	if err != nil {
		return fmt.Errorf("could not dial remote: %v", err)
	}

	localAddr := t.LocalAddress
	if !strings.Contains(t.LocalAddress, ":") {
		localAddr = "localhost:" + localAddr
	}
	t.listener, err = net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("can not listen locally: %v", err)
	}

	if t.stop == nil {
		t.stop = make(chan struct{})
		t.Closed = make(chan struct{})
	}

	go t.watch()

	go t.handleConnections()

	t.Status = Open
	return nil
}

func (t *Tunnel) watch() {
	clientClosed := make(chan struct{})
	go func() {
		t.client.Wait()
		t.listener.Close()
		clientClosed <- struct{}{}
	}()

	select {
	case <-clientClosed:
		if err := t.reconnectLoop(); err != nil {
			t.Status = Closed
			t.Closed <- struct{}{}
		}
	case <-t.stop:
		t.client.Close() // Will automatically close listener
		t.Status = Closed
		t.Closed <- struct{}{}
	}
}

func (t *Tunnel) reconnectLoop() error {
	t.Status = Reconnecting
	timeout := time.After(RECONNECT_TIMEOUT)
	wait := time.NewTimer(0.) // First time try immediately
	waitTime := RECONNECT_WAIT

	for {
		select {
		case <-timeout:
			return fmt.Errorf("reconnect timeout")
		case <-t.stop:
			return fmt.Errorf("reconnect interrupted")
		case <-wait.C:
			log.Infof("Reconnecting tunnel %v...", t.Name)
			err := t.Open()
			if err == nil {
				break
			}
			log.Errorf("could not reconnect tunnel %v: %v\nRetrying in %v...",
				t.Name, err, waitTime)
			wait.Reset(waitTime)
			waitTime *= 2
		}
	}
}

func (t *Tunnel) handleConnections() {
	defer t.listener.Close()
	defer t.client.Close()

	for {
		local, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept: %v", err)
			return
		}

		remote, err := t.client.Dial("tcp", t.RemoteAddress)
		if err != nil {
			log.Errorf("could not connect on remote: %v", err)
			return
		}

		runTunnel(local, remote)
	}
}

func runTunnel(local, remote net.Conn) {
	defer local.Close()
	defer remote.Close()
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(local, remote)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(remote, local)
		done <- struct{}{}
	}()

	<-done
}

func (t *Tunnel) Close() error {
	if t.Status == Closed {
		return fmt.Errorf("trying to close a closed tunnel")
	}
	t.stop <- struct{}{}
	return nil
}

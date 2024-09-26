package tunnel

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/alebeck/boring/internal/log"
	"golang.org/x/crypto/ssh"
)

const (
	RECONNECT_WAIT    = 2 * time.Millisecond
	RECONNECT_TIMEOUT = 10 * time.Minute
)

// Tunnel represents an SSH tunnel configuration and management
type Tunnel struct {
	Name          string        `toml:"name" json:"name"`
	LocalAddress  string        `toml:"local" json:"local"`
	RemoteAddress string        `toml:"remote" json:"remote"`
	Host          string        `toml:"host" json:"host"`
	User          string        `toml:"user" json:"user"`
	IdentityFile  string        `toml:"identity" json:"identity"`
	Port          int           `toml:"port" json:"port"`
	Mode          Mode          `toml:"mode" json:"mode"`
	Status        Status        `toml:"-" json:"status"`
	Closed        chan struct{} `toml:"-" json:"-"`
	rc            *runConfig    `toml:"-" json:"-"`
	client        *ssh.Client   `toml:"-" json:"-"`
	listener      net.Listener  `toml:"-" json:"-"`
	stop          chan struct{} `toml:"-" json:"-"`
}

func (t *Tunnel) Open() error {
	var err error
	if t.rc == nil {
		if err := t.makeRunConfig(); err != nil {
			return fmt.Errorf("could not make run config: %v", err)
		}
	}

	if err = t.setupClient(); err != nil {
		return fmt.Errorf("could not setup SSH client: %v", err)
	}

	if err = t.setupListener(); err != nil {
		return fmt.Errorf("cannot listen: %v", err)
	}

	if t.stop == nil {
		t.stop = make(chan struct{})
		t.Closed = make(chan struct{})
	}

	go t.watch()
	go t.handleConns()

	log.Infof("Opened tunnel %v...", t.Name)
	t.Status = Open
	return nil
}

func (t *Tunnel) setupClient() error {
	var err error
	addr := fmt.Sprintf("%v:%v", t.rc.hostName, t.rc.port)
	t.client, err = ssh.Dial("tcp", addr, t.rc.clientConfig)
	return err
}

func (t *Tunnel) setupListener() error {
	var err error
	if t.Mode == Local {
		t.listener, err = net.Listen(t.rc.localNet, t.rc.localAddress)
	} else {
		t.listener, err = t.client.Listen(t.rc.remoteNet, t.rc.remoteAddress)
	}
	return err
}

func (t *Tunnel) handleConns() {
	if t.Mode == Local {
		t.handleLocalConns()
	} else {
		t.handleRemoteConns()
	}
}

func (t *Tunnel) handleLocalConns() {
	defer t.listener.Close()
	defer t.client.Close()

	for {
		// Only handle one connection at a time
		local, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept: %v", err)
			return
		}

		remote, err := t.client.Dial(t.rc.remoteNet, t.rc.remoteAddress)
		if err != nil {
			log.Errorf("could not connect on remote: %v", err)
			return
		}

		runTunnel(local, remote)
	}
}

func (t *Tunnel) handleRemoteConns() {
	defer t.listener.Close()
	defer t.client.Close()

	for {
		remote, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept on remote: %v", err)
			return
		}
		go func() {
			local, err := net.Dial(t.rc.localNet, t.rc.localAddress)
			if err != nil {
				log.Errorf("could not dial locally: %v", err)
				return
			}
			runTunnel(local, remote)
		}()
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

func (t *Tunnel) watch() {
	clientClosed := make(chan struct{}, 1)
	go func() {
		t.client.Wait()
		t.listener.Close()
		clientClosed <- struct{}{}
	}()

	select {
	case <-clientClosed:
		if err := t.reconnectLoop(); err != nil {
			t.Status = Closed
			close(t.Closed)
		}
	case <-t.stop:
		log.Infof("Received stop signal for %v...", t.Name)
		t.client.Close() // Will also close listener
		t.Status = Closed
		close(t.Closed)
	}
}

func (t *Tunnel) reconnectLoop() error {
	t.Status = Reconn
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
				return nil
			}
			log.Errorf("could not reconnect tunnel %v: %v. Retrying in %v...",
				t.Name, err, waitTime)
			wait.Reset(waitTime)
			waitTime *= 2
		}
	}
}

func (t *Tunnel) Close() error {
	if t.Status == Closed {
		return fmt.Errorf("trying to close a closed tunnel")
	}
	t.stop <- struct{}{}
	return nil
}

package tunnel

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/alebeck/boring/internal/log"
	"golang.org/x/crypto/ssh"
)

const (
	reconnectWait     = 2 * time.Millisecond
	reconnectTimeout  = 10 * time.Minute
	keepAliveInterval = 2 * time.Minute
)

type Tunnel struct {
	Name          string         `toml:"name" json:"name"`
	LocalAddress  StringOrInt    `toml:"local" json:"local"`
	RemoteAddress string         `toml:"remote" json:"remote"`
	Host          string         `toml:"host" json:"host"`
	User          string         `toml:"user" json:"user"`
	IdentityFile  string         `toml:"identity" json:"identity"`
	Port          int            `toml:"port" json:"port"`
	Mode          Mode           `toml:"mode" json:"mode"`
	Status        Status         `toml:"-" json:"status"`
	Closed        chan struct{}  `toml:"-" json:"-"`
	rc            *runConfig     `toml:"-" json:"-"`
	client        *ssh.Client    `toml:"-" json:"-"`
	listener      net.Listener   `toml:"-" json:"-"`
	stop          chan struct{}  `toml:"-" json:"-"`
	wg            sync.WaitGroup `toml:"-" json:"-"`
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

	go t.run()

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

func (t *Tunnel) run() {
	disconn := make(chan struct{})
	go func() {
		t.client.Wait()
		close(disconn)
	}()

	go t.waitFor(func() { t.keepAlive(disconn) })
	go t.waitFor(func() { t.handleConns() })

	stopped := false
	select {
	case <-t.stop:
		log.Infof("Received stop signal for %v...", t.Name)
		stopped = true
		t.client.Close()
	case <-disconn:
	}
	t.listener.Close()
	t.wg.Wait()
	if !stopped && t.reconnectLoop() == nil {
		return
	}
	t.Status = Closed
	close(t.Closed)
}

func (t *Tunnel) keepAlive(cancel chan struct{}) {
	for {
		select {
		case <-cancel:
			return
		case <-time.After(keepAliveInterval):
			_, _, err := t.client.SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				log.Errorf("Error sending keepalive for tunnel %v: %v", t.Name, err)
				// Close the client, this triggers the reconnection logic
				t.client.Close()
				return
			}
			log.Debugf("Sent keep-alive")
		}
	}
}

func (t *Tunnel) handleConns() {
	defer t.listener.Close()
	defer t.client.Close()

	if t.Mode == Local {
		t.handleLocalConns()
	} else {
		t.handleRemoteConns()
	}
}

func (t *Tunnel) handleLocalConns() {
	for {
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

		go t.waitFor(func() { t.tunnel(local, remote) })
	}
}

func (t *Tunnel) handleRemoteConns() {
	for {
		remote, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept on remote: %v", err)
			return
		}
		go t.waitFor(func() {
			local, err := net.Dial(t.rc.localNet, t.rc.localAddress)
			if err != nil {
				log.Errorf("could not dial locally: %v", err)
				return
			}
			t.tunnel(local, remote)
		})
	}
}

func (t *Tunnel) tunnel(local, remote net.Conn) {
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

func (t *Tunnel) reconnectLoop() error {
	t.Status = Reconn
	timeout := time.After(reconnectTimeout)
	wait := time.NewTimer(0.) // First time try immediately
	waitTime := reconnectWait

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
	close(t.stop)
	return nil
}

// Logic registered with waitFor will be waited for upon tunnel closing
// and reconnecting.
func (t *Tunnel) waitFor(f func()) {
	t.wg.Add(1)
	defer t.wg.Done()
	f()
}

package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/proxy"
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
	RemoteAddress StringOrInt    `toml:"remote" json:"remote"`
	Host          string         `toml:"host" json:"host"`
	User          string         `toml:"user" json:"user"`
	IdentityFile  string         `toml:"identity" json:"identity"`
	Port          int            `toml:"port" json:"port"`
	Mode          Mode           `toml:"mode" json:"mode"`
	Status        Status         `toml:"-" json:"status"`
	LastConn      time.Time      `toml:"-" json:"last_conn"`
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

	if err = t.makeClient(); err != nil {
		return fmt.Errorf("could not setup SSH client: %v", err)
	}
	log.Debugf("%v: connected to server", t.Name)

	if err = t.makeListener(); err != nil {
		return fmt.Errorf("cannot listen: %v", err)
	}
	log.Debugf("%v: listening on %v", t.Name, t.listener.Addr())

	if t.stop == nil {
		t.stop = make(chan struct{})
		t.Closed = make(chan struct{})
	}

	go t.run()

	log.Infof("Opened tunnel %v...", t.Name)
	t.Status = Open
	t.LastConn = time.Now()
	return nil
}

func (t *Tunnel) makeClient() error {
	var err error
	addr := fmt.Sprintf("%v:%v", t.rc.hostName, t.rc.port)
	t.client, err = ssh.Dial("tcp", addr, t.rc.clientConfig)
	return err
}

func (t *Tunnel) makeListener() (err error) {
	if t.Mode == Remote || t.Mode == RemoteSocks {
		t.listener, err = t.client.Listen(t.rc.remoteNet, t.rc.remoteAddress)
	} else {
		t.listener, err = net.Listen(t.rc.localNet, t.rc.localAddress)
	}
	return
}

func (t *Tunnel) dial(network, addr string) (net.Conn, error) {
	if t.Mode == Remote || t.Mode == RemoteSocks {
		return net.Dial(network, addr)
	}
	return t.client.Dial(network, addr)
}

func (t *Tunnel) run() {
	disconn := make(chan struct{})
	go func() {
		t.client.Wait()
		log.Infof("Client closed for %v", t.Name)
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
	if t.Mode == Local || t.Mode == Remote {
		t.handleForward()
		return
	}
	t.handleSocks()
}

func (t *Tunnel) handleForward() {
	for {
		conn1, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept: %v", err)
			return
		}
		go t.waitFor(func() {
			netw, addr := t.rc.remoteNet, t.rc.remoteAddress
			if t.Mode == Remote || t.Mode == RemoteSocks {
				netw, addr = t.rc.localNet, t.rc.localAddress
			}
			conn2, err := t.dial(netw, addr)
			if err != nil {
				log.Errorf("could not dial: %v", err)
				return
			}
			tunnel(conn1, conn2)
		})
	}
}

func tunnel(c1, c2 net.Conn) {
	defer c1.Close()
	defer c2.Close()
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(c1, c2)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(c2, c1)
		done <- struct{}{}
	}()

	<-done
}

func (t *Tunnel) handleSocks() {
	serv := &proxy.Server{
		Dialer: func(ctx context.Context, netw, addr string) (net.Conn, error) {
			return t.dial(netw, addr)
		},
	}
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			log.Errorf("could not accept: %v", err)
			return
		}
		go t.waitFor(func() { serv.ServeConn(conn) })
	}
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

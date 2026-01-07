package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/proxy"
	"github.com/alebeck/boring/internal/ssh_config"
	"golang.org/x/crypto/ssh"
)

const (
	initReconnectWait = 500 * time.Millisecond
	maxReconnectWait  = 1 * time.Minute
	reconnectTimeout  = 15 * time.Minute
)

// Desc describes a tunnel for user-facing purposes, e.g., in the config file
// and in the TUI.
type Desc struct {
	Name          string      `toml:"name" json:"name"`
	LocalAddress  StringOrInt `toml:"local" json:"local"`
	RemoteAddress StringOrInt `toml:"remote" json:"remote"`
	Host          string      `toml:"host" json:"host"`
	User          string      `toml:"user" json:"user"`
	IdentityFile  string      `toml:"identity" json:"identity"`
	Port          int         `toml:"port" json:"port"`
	KeepAlive     *int        `toml:"keep_alive" json:"keep_alive"`
	Mode          Mode        `toml:"mode" json:"mode"`
	Group         string      `toml:"group" json:"group"`
	Status        Status      `toml:"-" json:"status"`
	LastConn      time.Time   `toml:"-" json:"last_conn"`
}

// Tunnel is a representation internal to the tunnel and daemon packages,
// describing a tunnel that is running or about to run.
type Tunnel struct {
	prepared   bool
	hops       []ssh_config.Hop
	Closed     chan struct{}
	stop       chan struct{}
	listener   net.Listener
	wg         sync.WaitGroup
	client     *ssh.Client
	localAddr  *address
	remoteAddr *address
	*Desc
}

type address struct {
	addr, net string
}

func FromDesc(desc *Desc) *Tunnel {
	return &Tunnel{Desc: desc}
}

func (t *Tunnel) Open() (err error) {
	if !t.prepared {
		if err = t.prepare(); err != nil {
			return err
		}
	}

	if err = t.makeClient(); err != nil {
		return fmt.Errorf("cannot make SSH client: %v", err)
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

	log.Infof("%v: opened tunnel", t.Name)
	t.Status = Open
	t.LastConn = time.Now()
	return
}

func (t *Tunnel) prepare() error {
	// We need to pass the user as it's needed for matching Match blocks
	sc, err := ssh_config.ParseSSHConfig(t.Host, t.User)
	if err != nil {
		return fmt.Errorf("could not parse SSH config: %v", err)
	}

	// Override values manually set by user
	if t.User != "" {
		sc.User = t.User
	}
	if t.Port != 0 {
		sc.Port = t.Port
	}
	if t.IdentityFile != "" {
		sc.IdentityFiles = []string{t.IdentityFile}
	}

	// If t.Host could not be resolved from ssh config, take it literally
	if sc.HostName == "" {
		sc.HostName = t.Host
	}

	sc.EnsureUser()

	// Infer series of hops from ssh config
	if t.hops, err = sc.ToHops(); err != nil {
		return err
	}

	allowShort := t.Mode == Remote || t.Mode == RemoteSocks
	t.remoteAddr, err = parseAddr(string(t.RemoteAddress), allowShort)
	if err != nil {
		return fmt.Errorf("remote address: %v", err)
	}

	t.localAddr, err = parseAddr(string(t.LocalAddress), !allowShort)
	if err != nil {
		return fmt.Errorf("local address: %v", err)
	}

	t.prepared = true

	return nil
}

func (t *Tunnel) makeClient() error {
	if len(t.hops) == 0 {
		return fmt.Errorf("no connections specified")
	}

	var c *ssh.Client
	var wg sync.WaitGroup

	// Connect through all jump hosts
	for _, j := range t.hops {
		addr := fmt.Sprintf("%v:%v", j.HostName, j.Port)
		n, err := wrapClient(c, addr, j.ClientConfig)
		if err != nil {
			safeClose(c)
			// Wait for all connections established until here to close
			wg.Wait()
			return fmt.Errorf("could not connect to host %v: %v", addr, err)
		}
		log.Debugf("%v: connected to host %v (client %p)", t.Name, j.HostName, n)

		// Add new client to wait group
		wg.Add(1)
		go func(n, c *ssh.Client) {
			defer wg.Done()
			n.Wait()
			log.Debugf("%v: closed client %p to %v", t.Name, n, n.RemoteAddr())
			// Close previous client when new one closes, this propagates
			safeClose(c)
		}(n, c)

		c = n
	}

	// Wait for all wrapped clients to close in case of tunnel closing or reconnection
	go t.waitFor(func() { wg.Wait() })

	t.client = c
	return nil
}

func wrapClient(old *ssh.Client, addr string, conf *ssh.ClientConfig) (*ssh.Client, error) {
	if old == nil {
		return ssh.Dial("tcp", addr, conf)
	}

	conn, err := old.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, conf)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(ncc, chans, reqs), nil
}

func (t *Tunnel) makeListener() (err error) {
	if t.Mode == Remote || t.Mode == RemoteSocks {
		t.listener, err = t.client.Listen(t.remoteAddr.net, t.remoteAddr.addr)
	} else {
		t.listener, err = net.Listen(t.localAddr.net, t.localAddr.addr)
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
		close(disconn)
	}()

	go t.waitFor(func() { t.keepAlive(disconn) })
	go t.waitFor(func() { t.handleConns() })

	stopped := false
	select {
	case <-t.stop:
		log.Infof("%v: received stop signal", t.Name)
		stopped = true
		t.client.Close()
	case <-disconn:
	}
	t.listener.Close()
	t.wg.Wait()
	if !stopped {
		if err := t.reconnectLoop(); err != nil {
			log.Errorf("%v: could not re-connect: %v", t.Name, err)
		} else {
			// Successfully re-connected
			return
		}
	}
	t.Status = Closed
	close(t.Closed)
}

func (t *Tunnel) keepAlive(cancel chan struct{}) {
	// panics if nil, this should never happen
	interv := *t.KeepAlive

	if interv == 0 {
		log.Infof("%v: disabling keep-alives since set to 0", t.Name)
		return
	}

	for {
		select {
		case <-cancel:
			return
		case <-time.After(time.Duration(interv) * time.Second):
			_, _, err := t.client.SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				log.Errorf("%v: error sending keepalive: %v", t.Name, err)
				// Close the client, this triggers the reconnection logic
				t.client.Close()
				return
			}
			log.Debugf("%v: sent keep-alive", t.Name)
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
			log.Errorf("%v: could not accept: %v", t.Name, err)
			return
		}
		go t.waitFor(func() {
			addr := t.remoteAddr
			if t.Mode == Remote || t.Mode == RemoteSocks {
				addr = t.localAddr
			}
			conn2, err := t.dial(addr.net, addr.addr)
			if err != nil {
				log.Errorf("%v: could not dial: %v", t.Name, err)
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
			log.Errorf("%v: could not accept: %v", t.Name, err)
			return
		}
		go t.waitFor(func() { serv.ServeConn(conn) })
	}
}

func (t *Tunnel) reconnectLoop() error {
	t.Status = Reconn
	timeout := time.After(reconnectTimeout)
	wait := time.NewTimer(2 * time.Millisecond) // First time try (essent.) immediately
	waitTime := initReconnectWait

	for {
		select {
		case <-timeout:
			return fmt.Errorf("re-connect timeout")
		case <-t.stop:
			return fmt.Errorf("re-connect interrupted by stop signal")
		case <-wait.C:
			log.Infof("%v: try re-connect...", t.Name)
			err := t.Open()
			if err == nil {
				return nil
			}
			log.Errorf("%v: could not re-connect: %v. Retrying in %v...",
				t.Name, err, waitTime)
			wait.Reset(waitTime)
			waitTime *= 2
			if waitTime > maxReconnectWait {
				waitTime = maxReconnectWait
			}
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

func parseAddr(addr string, allowShort bool) (*address, error) {
	if _, err := strconv.Atoi(addr); err == nil {
		// addr is a tcp port number
		if !allowShort {
			return nil, fmt.Errorf("bad remote forwarding specification")
		}
		return &address{"localhost:" + addr, "tcp"}, nil
	} else if strings.Contains(addr, ":") {
		// addr is a full tcp address
		return &address{addr, "tcp"}, nil
	}
	// it's a unix socket address
	return &address{addr, "unix"}, nil
}

func safeClose(c *ssh.Client) {
	if c != nil {
		c.Close()
	}
}

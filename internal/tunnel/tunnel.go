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
	Port          StringOrInt `toml:"port" json:"port"`
	KeepAlive     *int        `toml:"keep_alive" json:"keep_alive"`
	Group         string      `toml:"group" json:"group"`
	Mode          Mode        `toml:"mode" json:"mode"`
	Status        Status      `toml:"-" json:"status"`
	LastConn      time.Time   `toml:"-" json:"last_conn"`
}

// Tunnel is a representation internal to the tunnel and daemon packages,
// describing a tunnel that is running or about to be run.
type Tunnel struct {
	prepared  bool
	hops      []ssh_config.Hop
	Closed    chan struct{}
	stop      chan struct{}
	stopOnce  sync.Once
	listeners []net.Listener
	wg        sync.WaitGroup
	client    *ssh.Client
	fwds      []forward
	// mu guards Status and LastConn, which the tunnel's own goroutines
	// update while the daemon may be reading them for a listing.
	mu sync.Mutex
	*Desc
}

type address struct {
	addr, net string
}

type forward struct {
	local, remote *address
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
		return err
	}
	log.Debugf("%v: connected to server", t.Name)

	if err = t.makeListeners(); err != nil {
		t.client.Close()
		return fmt.Errorf("cannot listen: %v", err)
	}

	if t.stop == nil {
		t.stop = make(chan struct{})
		t.Closed = make(chan struct{})
	}

	t.mu.Lock()
	t.Status = Open
	t.LastConn = time.Now()
	t.mu.Unlock()

	go t.run()

	log.Infof("%v: opened tunnel", t.Name)
	return
}

// Snapshot returns a copy of the tunnel's description that is safe to take
// while the tunnel is running.
func (t *Tunnel) Snapshot() Desc {
	t.mu.Lock()
	defer t.mu.Unlock()
	return *t.Desc
}

func (t *Tunnel) setStatus(s Status) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = s
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
	if t.Port != "" {
		if sc.Port, err = strconv.Atoi(t.Port.String()); err != nil {
			return fmt.Errorf("invalid port %q", t.Port)
		}
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
	remotes, err := parseAddrs(string(t.RemoteAddress), allowShort)
	if err != nil {
		return fmt.Errorf("remote address: %v", err)
	}

	locals, err := parseAddrs(string(t.LocalAddress), !allowShort)
	if err != nil {
		return fmt.Errorf("local address: %v", err)
	}

	if len(locals) != len(remotes) {
		return fmt.Errorf("local and remote have different numbers of ports (%d vs %d)",
			len(locals), len(remotes))
	}
	t.fwds = make([]forward, len(locals))
	for i := range locals {
		t.fwds[i] = forward{locals[i], remotes[i]}
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
	t.goWait(wg.Wait)

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

// makeListeners listens on one address per forward. If any listen fails,
// those that succeeded are closed again.
func (t *Tunnel) makeListeners() error {
	t.listeners = make([]net.Listener, 0, len(t.fwds))
	for _, f := range t.fwds {
		var l net.Listener
		var err error
		if t.Mode == Remote || t.Mode == RemoteSocks {
			l, err = t.client.Listen(f.remote.net, f.remote.addr)
			if err != nil {
				err = fmt.Errorf("%v: %v", f.remote.addr, err)
			}
		} else {
			l, err = net.Listen(f.local.net, f.local.addr)
		}
		if err != nil {
			t.closeListeners()
			return err
		}
		log.Debugf("%v: listening on %v", t.Name, l.Addr())
		t.listeners = append(t.listeners, l)
	}
	return nil
}

func (t *Tunnel) closeListeners() {
	for _, l := range t.listeners {
		l.Close()
	}
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

	t.goWait(func() { t.keepAlive(disconn) })
	for i, l := range t.listeners {
		t.goWait(func() { t.handleConns(l, t.fwds[i]) })
	}

	stopped := false
	select {
	case <-t.stop:
		log.Infof("%v: received stop signal", t.Name)
		stopped = true
		t.client.Close()
	case <-disconn:
	}
	t.closeListeners()
	t.wg.Wait()
	if !stopped {
		if err := t.reconnectLoop(); err != nil {
			log.Errorf("%v: could not re-connect: %v", t.Name, err)
		} else {
			// Successfully re-connected
			return
		}
	}
	t.setStatus(Closed)
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

func (t *Tunnel) handleConns(l net.Listener, f forward) {
	defer l.Close()
	defer t.client.Close()
	if t.Mode == Local || t.Mode == Remote {
		t.handleForward(l, f)
		return
	}
	t.handleSocks(l)
}

func (t *Tunnel) handleForward(l net.Listener, f forward) {
	for {
		conn1, err := l.Accept()
		if err != nil {
			log.Errorf("%v: could not accept: %v", t.Name, err)
			return
		}
		t.goWait(func() {
			addr := f.remote
			if t.Mode == Remote || t.Mode == RemoteSocks {
				addr = f.local
			}
			conn2, err := t.dial(addr.net, addr.addr)
			if err != nil {
				log.Errorf("%v: could not dial: %v", t.Name, err)
				conn1.Close()
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

func (t *Tunnel) handleSocks(l net.Listener) {
	serv := &proxy.Server{
		Dialer: func(ctx context.Context, netw, addr string) (net.Conn, error) {
			return t.dial(netw, addr)
		},
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorf("%v: could not accept: %v", t.Name, err)
			return
		}
		t.goWait(func() { serv.ServeConn(conn) })
	}
}

func (t *Tunnel) reconnectLoop() error {
	t.setStatus(Reconn)
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

// Close signals the tunnel to stop. It is safe to call concurrently and
// more than once; wait on Closed for the tunnel to actually shut down.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	closed := t.Status == Closed
	t.mu.Unlock()
	if closed {
		return fmt.Errorf("trying to close a closed tunnel")
	}
	t.stopOnce.Do(func() { close(t.stop) })
	return nil
}

// goWait runs f in a new goroutine that will be waited for upon tunnel
// closing and reconnecting. The wait group is incremented before the
// goroutine starts, so a concurrent Wait cannot miss it.
func (t *Tunnel) goWait(f func()) {
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		f()
	}()
}

// parseAddrs parses an address whose port may be a list of ports and
// ranges, e.g. "localhost:8000-8010,8080", into one address per port.
func parseAddrs(addr string, allowShort bool) ([]*address, error) {
	host, spec := "localhost", addr
	if strings.Trim(addr, "0123456789,-") != "" {
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			// it's a unix socket address
			return []*address{{addr, "unix"}}, nil
		}
		host, spec = addr[:i], addr[i+1:]
	} else if !allowShort {
		return nil, fmt.Errorf("bad remote forwarding specification")
	}
	ports, err := parsePorts(spec)
	if err != nil {
		return nil, err
	}
	addrs := make([]*address, len(ports))
	for i, p := range ports {
		addrs[i] = &address{host + ":" + p, "tcp"}
	}
	return addrs, nil
}

// parsePorts expands a list of ports and ranges like "8000-8010,8080".
func parsePorts(spec string) ([]string, error) {
	var ports []string
	for item := range strings.SplitSeq(spec, ",") {
		a, b, isRange := strings.Cut(item, "-")
		if !isRange {
			b = a
		}
		lo, err1 := strconv.Atoi(a)
		hi, err2 := strconv.Atoi(b)
		if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
			return nil, fmt.Errorf("invalid port %q", item)
		}
		for p := lo; p <= hi; p++ {
			ports = append(ports, strconv.Itoa(p))
		}
	}
	return ports, nil
}

func safeClose(c *ssh.Client) {
	if c != nil {
		c.Close()
	}
}

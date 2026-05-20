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

	"github.com/alebeck/boring/internal/auth"
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
	User          string      `toml:"user,omitempty" json:"user"`
	IdentityFile  string      `toml:"identity,omitempty" json:"identity"`
	// Port is a pointer so an unset port is omitted from written TOML.
	// BurntSushi/toml's omitempty does not skip numeric zero values, so a
	// plain int field of 0 would still be written as "port = 0".
	Port      *int      `toml:"port,omitempty" json:"port"`
	KeepAlive *int      `toml:"keep_alive,omitempty" json:"keep_alive"`
	Group     string    `toml:"group,omitempty" json:"group"`
	Mode      Mode      `toml:"mode" json:"mode"`
	Status    Status    `toml:"-" json:"status"`
	LastConn  time.Time `toml:"-" json:"last_conn"`
	// Forwards is the multi-forward model: every loaded tunnel has at least
	// one forward. The toml:"forward" tag makes [[tunnels.forward]] blocks
	// decode into this slice; config.Load also folds the legacy
	// local/remote/mode shorthand into a single-element Forwards slice.
	Forwards []Forward `toml:"forward,omitempty" json:"forwards,omitempty"`
}

// Tunnel is a representation internal to the tunnel and daemon packages,
// describing a tunnel that is running or about to be run.
type Tunnel struct {
	prepared bool
	// interactive records that the tunnel authenticated via keyboard-interactive
	// (2FA), so it must not be silently auto-reconnected: a fresh code is required
	// and the daemon cannot prompt non-interactively.
	interactive bool
	// prompter supplies interactive auth answers (2FA codes, key passphrases).
	// A nil prompter means non-interactive.
	prompter auth.Prompter
	hops     []ssh_config.Hop
	Closed   chan struct{}
	stop     chan struct{}
	wg       sync.WaitGroup
	client   *ssh.Client
	// forwards holds one runtime entry per Forward: a tunnel owns a single SSH
	// connection (client) and one listener per forward. Populated by prepare().
	forwards []*forwardRuntime
	*Desc
}

type address struct {
	addr, net string
}

// interactivePrompter wraps a Prompter and records on the tunnel when a
// keyboard-interactive (2FA) challenge is answered, as distinct from a
// key-passphrase prompt. A tunnel that authenticated with 2FA cannot be
// silently auto-reconnected, since a fresh code is required each time.
//
// The two are told apart by the prompt name: a server-supplied
// keyboard-interactive challenge that happens to be named exactly
// auth.PassphrasePromptName would be misclassified as a passphrase prompt.
// That edge case is a deliberate, benign trade-off — the only consequence is
// a blind reconnect attempt that fails auth rather than resting at NeedsAuth.
type interactivePrompter struct {
	inner  auth.Prompter
	tunnel *Tunnel
}

func (p *interactivePrompter) Prompt(name, instruction string,
	questions []string, echo []bool) ([]string, error) {
	if name != auth.PassphrasePromptName {
		p.tunnel.interactive = true
	}
	return p.inner.Prompt(name, instruction, questions, echo)
}

// FromDesc builds a Tunnel from a description. prompter supplies interactive
// auth answers (2FA codes, key passphrases); pass nil for non-interactive use.
func FromDesc(desc *Desc, prompter auth.Prompter) *Tunnel {
	return &Tunnel{Desc: desc, prompter: prompter}
}

func (t *Tunnel) Open() (err error) {
	// prepare() resolves SSH config and loads keys, decrypting any
	// passphrase-protected ones (prompting the user). The prepared guard
	// makes this run exactly once: reconnect attempts reuse the decrypted
	// signers cached in t.hops, so the user is never re-prompted for a key
	// passphrase on reconnect.
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
	if t.Port != nil {
		sc.Port = *t.Port
	}
	if t.IdentityFile != "" {
		sc.IdentityFiles = []string{t.IdentityFile}
	}

	// If t.Host could not be resolved from ssh config, take it literally
	if sc.HostName == "" {
		sc.HostName = t.Host
	}

	sc.EnsureUser()

	// Infer series of hops from ssh config. The tunnel's prompter (which may be
	// nil for non-interactive use) reaches SSH auth through here. It is wrapped
	// so that answering a keyboard-interactive (2FA) challenge marks the tunnel
	// interactive, which suppresses blind auto-reconnect.
	var prompter auth.Prompter
	if t.prompter != nil {
		prompter = &interactivePrompter{inner: t.prompter, tunnel: t}
	}
	if t.hops, err = sc.ToHops(prompter); err != nil {
		return err
	}

	// Address parsing is per-forward: each Forward carries its own
	// local/remote addresses and mode. The connection-level resolution above
	// runs once for the whole tunnel.
	if err = t.prepareForwards(); err != nil {
		return err
	}

	t.prepared = true

	return nil
}

// prepareForwards builds the per-forward runtime slice, parsing every Forward's
// local/remote addresses for its own mode. Every loaded tunnel has at least one
// forward; an empty Forwards slice is a programming error.
func (t *Tunnel) prepareForwards() error {
	if len(t.Forwards) == 0 {
		return fmt.Errorf("tunnel has no forwards")
	}
	t.forwards = make([]*forwardRuntime, 0, len(t.Forwards))
	for _, f := range t.Forwards {
		fr, err := parseForward(f)
		if err != nil {
			return fmt.Errorf("forward %q: %v", f.Label(), err)
		}
		t.forwards = append(t.forwards, fr)
	}
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

// makeListeners binds one listener per forward. It is atomic: if any forward's
// listener fails to bind, every listener already created in this call is closed
// and an error naming the offending forward is returned, so a tunnel never
// opens partially.
func (t *Tunnel) makeListeners() error {
	for _, fr := range t.forwards {
		if err := t.makeListener(fr); err != nil {
			t.closeListeners()
			return fmt.Errorf("forward %q: %v", fr.Label(), err)
		}
		log.Debugf("%v: forward %q listening on %v",
			t.Name, fr.Label(), fr.listener.Addr())
	}
	return nil
}

// makeListener binds a single forward's listener: a remote listener requested
// from the server for remote/socks-remote modes, a local socket otherwise.
func (t *Tunnel) makeListener(fr *forwardRuntime) (err error) {
	if fr.isRemote() {
		fr.listener, err = t.client.Listen(fr.remoteAddr.net, fr.remoteAddr.addr)
	} else {
		fr.listener, err = net.Listen(fr.localAddr.net, fr.localAddr.addr)
	}
	return
}

// closeListeners closes every forward listener that is currently open.
func (t *Tunnel) closeListeners() {
	for _, fr := range t.forwards {
		if fr.listener != nil {
			fr.listener.Close()
			fr.listener = nil
		}
	}
}

func (t *Tunnel) dial(fr *forwardRuntime, network, addr string) (net.Conn, error) {
	if fr.isRemote() {
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

	// Keep-alive runs once for the single shared client; one connection
	// handler runs per forward.
	go t.waitFor(func() { t.keepAlive(disconn) })
	for _, fr := range t.forwards {
		go t.waitFor(func() { t.handleConns(fr) })
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
	if t.shouldReconnect(stopped) {
		if err := t.reconnectLoop(); err != nil {
			log.Errorf("%v: could not re-connect: %v", t.Name, err)
		} else {
			// Successfully re-connected
			return
		}
	}
	t.Status = t.finalStatus(stopped)
	close(t.Closed)
}

// shouldReconnect reports whether run() should attempt reconnection after an
// unexpected disconnect. Interactive (2FA) tunnels cannot: a fresh code is
// required and the daemon cannot prompt non-interactively.
func (t *Tunnel) shouldReconnect(stopped bool) bool {
	return !stopped && !t.interactive
}

// finalStatus is the status a tunnel rests at once run() exits without
// reconnecting: NeedsAuth for an interactive tunnel that dropped unexpectedly
// (it needs the user to re-authenticate), Closed otherwise.
func (t *Tunnel) finalStatus(stopped bool) Status {
	if !stopped && t.interactive {
		return NeedsAuth
	}
	return Closed
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

func (t *Tunnel) handleConns(fr *forwardRuntime) {
	defer fr.listener.Close()
	defer t.client.Close()
	if fr.isSocks() {
		t.handleSocks(fr)
		return
	}
	t.handleForward(fr)
}

func (t *Tunnel) handleForward(fr *forwardRuntime) {
	for {
		conn1, err := fr.listener.Accept()
		if err != nil {
			log.Errorf("%v: forward %q could not accept: %v",
				t.Name, fr.Label(), err)
			return
		}
		go t.waitFor(func() {
			addr := fr.remoteAddr
			if fr.isRemote() {
				addr = fr.localAddr
			}
			conn2, err := t.dial(fr, addr.net, addr.addr)
			if err != nil {
				log.Errorf("%v: forward %q could not dial: %v",
					t.Name, fr.Label(), err)
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

func (t *Tunnel) handleSocks(fr *forwardRuntime) {
	serv := &proxy.Server{
		Dialer: func(ctx context.Context, netw, addr string) (net.Conn, error) {
			return t.dial(fr, netw, addr)
		},
	}
	for {
		conn, err := fr.listener.Accept()
		if err != nil {
			log.Errorf("%v: forward %q could not accept: %v",
				t.Name, fr.Label(), err)
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

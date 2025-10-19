package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/alebeck/boring/internal/buildinfo"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

const (
	Flag        = "--daemon"
	sockName    = "boringd.sock"
	logFileName = "boringd.log"
)

var (
	LogFile        string
	Socket         string
	AlreadyRunning = errors.New("already running")
)

func init() {
	if LogFile = os.Getenv("BORING_LOG_FILE"); LogFile == "" {
		LogFile = filepath.Join(os.TempDir(), logFileName)
	}
	if Socket = os.Getenv("BORING_SOCK"); Socket == "" {
		Socket = filepath.Join(os.TempDir(), sockName)
	}
}

type daemon struct {
	ctx    context.Context
	cancel context.CancelFunc
	ln     net.Listener

	// TODO: write proper concurrent map structure for this
	tunnels map[string]*tunnel.Tunnel
	mutex   sync.RWMutex

	once sync.Once
	wg   sync.WaitGroup
}

func newDaemon(parent context.Context, ln net.Listener) (*daemon, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	tunnels := make(map[string]*tunnel.Tunnel)
	d := &daemon{ctx: ctx, cancel: cancel, ln: ln, tunnels: tunnels}

	go func() {
		// Parent-driven shutdown
		<-parent.Done()
		log.Infof("Received signal: %v", parent.Err())
		d.stop()
	}()

	cleanup := func() {
		log.Infof("Cleaning up...")
		d.stop()
		d.wg.Wait()

		// Take snapshot of tunnels to close
		d.mutex.Lock()
		ts := make([]*tunnel.Tunnel, 0, len(d.tunnels))
		for _, t := range d.tunnels {
			ts = append(ts, t)
		}
		d.mutex.Unlock()

		// Drain
		for _, t := range ts {
			t.Close()
		}
		for _, t := range ts {
			<-t.Closed
		}
		log.Infof("Done.")
	}
	return d, cleanup
}

func respond(conn net.Conn, opErr error, ts map[string]tunnel.Desc) {
	resp := Resp{Success: true, Tunnels: ts, Info: Info{Commit: buildinfo.Commit}}
	if opErr != nil {
		resp.Success = false
		resp.Error = opErr.Error()
	}
	if err := ipc.Write(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func (d *daemon) handleConn(conn net.Conn) {
	defer conn.Close()

	// Tie deadline to context cancel
	done := make(chan struct{})
	go func() {
		select {
		case <-d.ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	defer close(done)

	// Read command
	var cmd Cmd
	if err := ipc.Read(&cmd, conn); err != nil {
		// Ignore cases where client aborts connection
		if !errors.Is(err, io.EOF) {
			log.Errorf("Could not receive command: %v", err)
		}
		return
	}
	log.Debugf("Received command %v", cmd)

	// Execute command
	switch cmd.Kind {
	case Nop:
		respond(conn, nil, nil)
	case Open:
		d.openTunnel(conn, &cmd.Tunnel)
	case Close:
		d.closeTunnel(conn, &cmd.Tunnel)
	case List:
		d.listTunnels(conn)
	case Shutdown:
		log.Infof("Shutdown command received.")
		respond(conn, nil, nil)
		d.stop()
	default:
		err := fmt.Errorf("unknown command: %v", cmd.Kind)
		respond(conn, err, nil)
	}
}

func (d *daemon) openTunnel(conn net.Conn, desc *tunnel.Desc) {
	var err error
	defer func() { respond(conn, err, nil) }()

	d.mutex.RLock()
	_, exists := d.tunnels[desc.Name]
	d.mutex.RUnlock()
	if exists {
		err = AlreadyRunning
		log.Errorf("%v: could not open: %v", desc.Name, err)
		return
	}

	t := tunnel.FromDesc(desc)
	if err = t.Open(); err != nil {
		log.Errorf("%v: could not open: %v", t.Name, err)
		return
	}

	d.mutex.Lock()
	d.tunnels[t.Name] = t
	d.mutex.Unlock()

	// Register closing logic
	go func() {
		<-t.Closed
		d.mutex.Lock()
		delete(d.tunnels, t.Name)
		d.mutex.Unlock()
		log.Infof("Closed tunnel %s", t.Name)
	}()
}

func (d *daemon) closeTunnel(conn net.Conn, q *tunnel.Desc) {
	var err error
	defer func() { respond(conn, err, nil) }()

	d.mutex.RLock()
	t, ok := d.tunnels[q.Name]
	d.mutex.RUnlock()
	if !ok {
		err = fmt.Errorf("tunnel not running")
		log.Errorf("%v: could not close tunnel: %v", q.Name, err)
		return
	}

	if err = t.Close(); err != nil {
		log.Errorf("%v: could not close tunnel: %v", t.Name, err)
		return
	}
	<-t.Closed
}

func (d *daemon) listTunnels(conn net.Conn) {
	d.mutex.RLock()
	ts := make(map[string]tunnel.Desc, len(d.tunnels))
	for n, t := range d.tunnels {
		ts[n] = *t.Desc
	}
	d.mutex.RUnlock()
	respond(conn, nil, ts)
}

func initLogging(path string) {
	logFile, err := os.OpenFile(
		path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.Init(logFile, true, runtime.GOOS != "windows")
}

func listen() (l net.Listener, err error) {
	l, err = net.Listen("unix", Socket)
	if err == nil {
		return
	}
	// If the daemon was terminated forcefully, the domain socket
	// may be in a bad state where it exists but doesn't allow binding.
	// We try to identify this and delete the socket file, if necessary.
	if _, statErr := os.Stat(Socket); statErr == nil {
		if _, dialErr := net.Dial("unix", Socket); dialErr != nil {
			log.Warningf("Found unresponsive socket, deleting...")
			os.Remove(Socket)
			l, err = net.Listen("unix", Socket)
		}
	}
	return
}

func (d *daemon) serve() {
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			if d.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Errorf("Failed to accept connection: %v", err)
			continue
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConn(conn)
		}()
	}
}

// stop breaks out of the Accept loop in serve.
func (d *daemon) stop() {
	d.once.Do(func() {
		log.Infof("Stopping.")
		d.cancel()
		d.ln.Close()
	})
}

func Run() {
	initLogging(LogFile)
	log.Infof("Daemon starting")

	ln, err := listen()
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}
	log.Infof("Listening on %s", ln.Addr())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	d, cleanup := newDaemon(ctx, ln)
	defer cleanup()

	d.serve()
}

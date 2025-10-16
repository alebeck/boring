package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

var AlreadyRunning = errors.New("already running")

type daemon struct {
	// TODO: write proper concurrent map structure for this
	tunnels map[string]*tunnel.Tunnel
	mutex   sync.RWMutex
	wg      sync.WaitGroup
}

func newDaemon() *daemon {
	return &daemon{
		tunnels: make(map[string]*tunnel.Tunnel),
	}
}

func (d *daemon) cleanup() {
	d.wg.Wait()
	d.mutex.Lock()
	for _, t := range d.tunnels {
		t.Close()
	}
	for _, t := range d.tunnels {
		<-t.Closed
	}
}

func respond(conn net.Conn, err *error) {
	resp := Resp{Success: true}
	if *err != nil {
		resp = Resp{Success: false, Error: (*err).Error()}
	}
	if err := ipc.Write(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func (d *daemon) handleConnection(conn net.Conn) {
	defer conn.Close()

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
	case Open:
		d.openTunnel(conn, &cmd.Tunnel)
	case Close:
		d.closeTunnel(conn, &cmd.Tunnel)
	case List:
		d.listTunnels(conn)
	default:
		err := fmt.Errorf("unknown command: %v", cmd.Kind)
		respond(conn, &err)
	}
}

func (d *daemon) openTunnel(conn net.Conn, desc *tunnel.Desc) {
	var err error
	defer respond(conn, &err)

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
	defer respond(conn, &err)

	d.mutex.RLock()
	t, ok := d.tunnels[q.Name]
	d.mutex.RUnlock()
	if !ok {
		err = fmt.Errorf("tunnel not running")
		log.Errorf("%v: could not close tunnel: %v", t.Name, err)
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
	m := make(map[string]tunnel.Desc, len(d.tunnels))
	for n, t := range d.tunnels {
		m[n] = *t.Desc
	}
	d.mutex.RUnlock()

	resp := Resp{Success: true, Tunnels: m}
	if err := ipc.Write(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func initLogging(path string) {
	logFile, err := os.OpenFile(
		path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.Init(logFile, true, runtime.GOOS != "windows")
}

func makeListener() (l net.Listener, err error) {
	l, err = net.Listen("unix", sock)
	if err == nil {
		return
	}
	// If the daemon was terminated forcefully, the domain socket
	// may be in a bad daemon where it exists but doesn't allow binding.
	// We try to identify this and delete the socket file, if necessary.
	if _, statErr := os.Stat(sock); statErr == nil {
		if _, dialErr := net.Dial("unix", sock); dialErr != nil {
			log.Warningf("Found unresponsive socket, deleting...")
			os.Remove(sock)
			l, err = net.Listen("unix", sock)
		}
	}
	return
}

func closeOnInterrupt(l net.Listener) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	log.Infof("Received signal: %s. Closing.", <-sig)
	l.Close()
}

func (d *daemon) run() {
	l, err := makeListener()
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}
	log.Infof("Listening on %s", l.Addr())

	go closeOnInterrupt(l)

	// Daemon control loop
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorf("Failed to accept connection: %v", err)
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.handleConnection(conn)
		}()
	}
}

func Run() {
	initLogging(logFile)
	log.Infof("Daemon starting")

	d := newDaemon()
	defer d.cleanup()

	d.run()
}

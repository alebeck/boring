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

type state struct {
	// TODO: write proper concurrent map structure for this
	tunnels map[string]*tunnel.Tunnel
	mutex   sync.RWMutex
}

func newState() *state {
	return &state{tunnels: make(map[string]*tunnel.Tunnel)}
}

func Run() {
	initLogging(logFile)
	log.Infof("Daemon starting")

	l, err := setupListener()
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}
	log.Infof("Listening on %s", l.Addr())

	go watchSignal(l)

	var wg sync.WaitGroup
	s := newState()
	defer cleanup(s, &wg)

	// Handle incoming connections
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorf("Failed to accept connection: %v", err)
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(s, conn)
		}()
	}
}

func watchSignal(l net.Listener) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	log.Infof("Received signal: %s. Closing.", <-sig)
	l.Close()
}

func cleanup(s *state, wg *sync.WaitGroup) {
	log.Infof("Cleaning up.")
	wg.Wait()
	s.mutex.Lock()
	for _, t := range s.tunnels {
		t.Close()
	}
	for _, t := range s.tunnels {
		<-t.Closed
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

func setupListener() (l net.Listener, err error) {
	l, err = net.Listen("unix", sock)
	if err == nil {
		return
	}
	// If the daemon was terminated forcefully, the domain socket
	// may be in a bad state where it exists but doesn't allow binding.
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

func handleConnection(s *state, conn net.Conn) {
	defer conn.Close()

	// Receive command
	var cmd Cmd
	if err := ipc.Receive(&cmd, conn); err != nil {
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
		openTunnel(s, conn, &cmd.Tunnel)
	case Close:
		closeTunnel(s, conn, &cmd.Tunnel)
	case List:
		listTunnels(s, conn)
	default:
		unknownCmd(conn, cmd.Kind)
	}
}

func respond(conn net.Conn, err *error) {
	resp := Resp{Success: true}
	if *err != nil {
		resp = Resp{Success: false, Error: (*err).Error()}
	}
	if err := ipc.Send(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func openTunnel(s *state, conn net.Conn, d *tunnel.TunnelDesc) {
	var err error
	defer respond(conn, &err)

	s.mutex.RLock()
	_, exists := s.tunnels[d.Name]
	s.mutex.RUnlock()
	if exists {
		err = AlreadyRunning
		log.Errorf("%v: could not open: %v", d.Name, err)
		return
	}

	t := tunnel.FromDesc(d)
	if err = t.Open(); err != nil {
		log.Errorf("%v: could not open: %v", t.Name, err)
		return
	}

	s.mutex.Lock()
	s.tunnels[t.Name] = t
	s.mutex.Unlock()

	// Register closing logic
	go func() {
		<-t.Closed
		s.mutex.Lock()
		delete(s.tunnels, t.Name)
		s.mutex.Unlock()
		log.Infof("Closed tunnel %s", t.Name)
	}()
}

func closeTunnel(s *state, conn net.Conn, q *tunnel.TunnelDesc) {
	var err error
	defer respond(conn, &err)

	s.mutex.RLock()
	t, ok := s.tunnels[q.Name]
	s.mutex.RUnlock()
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

func listTunnels(s *state, conn net.Conn) {
	m := make(map[string]tunnel.TunnelDesc)
	s.mutex.RLock()
	for n, t := range s.tunnels {
		m[n] = *t.TunnelDesc
	}
	s.mutex.RUnlock()

	resp := Resp{Success: true, Tunnels: m}
	if err := ipc.Send(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func unknownCmd(conn net.Conn, k CmdKind) {
	err := fmt.Errorf("unknown command: %v", k)
	respond(conn, &err)
}

package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

type state struct {
	// TODO: write proper concurrent map structure for this
	tunnels map[string]*tunnel.Tunnel
	mutex   sync.RWMutex
}

func newState() *state {
	return &state{tunnels: make(map[string]*tunnel.Tunnel)}
}

func Run() {
	setupLogger(logFile)
	log.Infof("Daemon starting")

	l, err := setupListener()
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}
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

func setupLogger(path string) {
	logFile, err := os.OpenFile(
		path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
}

func setupListener() (net.Listener, error) {
	return net.Listen("unix", sock)
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
	log.Infof("Received command %v", cmd)

	// Execute command
	switch cmd.Kind {
	case Open:
		openTunnel(s, conn, cmd.Tunnel)
	case Close:
		closeTunnel(s, conn, cmd.Tunnel)
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

func openTunnel(s *state, conn net.Conn, t tunnel.Tunnel) {
	var err error
	defer respond(conn, &err)

	s.mutex.RLock()
	_, exists := s.tunnels[t.Name]
	s.mutex.RUnlock()
	if exists {
		err = fmt.Errorf("tunnel already running")
		return
	}

	if err = t.Open(); err != nil {
		return
	}

	s.mutex.Lock()
	s.tunnels[t.Name] = &t
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

func closeTunnel(s *state, conn net.Conn, q tunnel.Tunnel) {
	var err error
	defer respond(conn, &err)

	s.mutex.RLock()
	t, ok := s.tunnels[q.Name]
	s.mutex.RUnlock()
	if !ok {
		err = fmt.Errorf("tunnel not running")
		return
	}

	if err = t.Close(); err != nil {
		err = fmt.Errorf("could not close tunnel: %v", err)
		return
	}
	<-t.Closed
}

func listTunnels(s *state, conn net.Conn) {
	m := make(map[string]tunnel.Tunnel)
	s.mutex.RLock()
	for n, t := range s.tunnels {
		m[n] = *t
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

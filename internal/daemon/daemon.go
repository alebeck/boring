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

// TODO: write proper concurrent map structure for this
var tunnels = make(map[string]*tunnel.Tunnel)
var mutex sync.RWMutex
var listener net.Listener

func Run() {
	setupLogger(logFile)
	log.Infof("Daemon starting")

	var err error
	if listener, err = setupListener(); err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}

	defer cleanup()
	go watchSignal()

	// Handle incoming connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Errorf("Failed to accept connection: %v", err)
			if errors.Is(err, net.ErrClosed) {
				return
			}
		}
		go handleConnection(conn)
	}
}

func watchSignal() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	log.Infof("Received signal: %s. Closing.", <-sig)
	listener.Close()
}

// Logic for socket cleanup on TERM/INT signal. On
// SIGKILL and other abrupt interruptions, the socket
// file is likely not cleaned up, and has to be deleted
// manually. TODO: Detect & fix this state in the CLI.
func cleanup() {
	log.Infof("Cleaning up.")
	listener.Close()
	mutex.Lock()
	for _, t := range tunnels {
		t.Close()
	}
	for _, t := range tunnels {
		<-t.Closed
	}
}

func setupLogger(path string) {
	logFile, err := os.OpenFile(path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
}

func setupListener() (net.Listener, error) {
	return net.Listen("unix", sock)
}

func handleConnection(conn net.Conn) {
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
		openTunnel(conn, cmd.Tunnel)
	case Close:
		closeTunnel(conn, cmd.Tunnel)
	case List:
		listTunnels(conn)
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

func openTunnel(conn net.Conn, t tunnel.Tunnel) {
	var err error
	defer respond(conn, &err)

	mutex.RLock()
	_, exists := tunnels[t.Name]
	mutex.RUnlock()
	if exists {
		err = fmt.Errorf("tunnel already running")
		return
	}

	if err = t.Open(); err != nil {
		return
	}

	mutex.Lock()
	tunnels[t.Name] = &t
	mutex.Unlock()

	// Register closing logic
	go func() {
		<-t.Closed
		mutex.Lock()
		delete(tunnels, t.Name)
		mutex.Unlock()
		log.Infof("Closed tunnel %s", t.Name)
	}()
}

func closeTunnel(conn net.Conn, q tunnel.Tunnel) {
	var err error
	defer respond(conn, &err)

	mutex.RLock()
	t, ok := tunnels[q.Name]
	mutex.RUnlock()
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

func listTunnels(conn net.Conn) {
	m := make(map[string]tunnel.Tunnel)
	mutex.RLock()
	for n, t := range tunnels {
		m[n] = *t
	}
	mutex.RUnlock()

	resp := Resp{Success: true, Tunnels: m}
	if err := ipc.Send(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func unknownCmd(conn net.Conn, k CmdKind) {
	err := fmt.Errorf("unknown command: %v", k)
	respond(conn, &err)
}

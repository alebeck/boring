package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alebeck/boring/internal/daemon"
	"github.com/alebeck/boring/internal/ipc"
	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
)

var tunnels = make(map[string]*tunnel.Tunnel)
var mutex sync.RWMutex

func main() {
	if len(os.Args) < 3 {
		log.Fatalf("Daemon called with args: %v", os.Args[1:])
	}

	initLogger(os.Args[2])
	log.Infof("Daemon starting with args: %v", os.Args[1:])

	l, err := setupListener()
	if err != nil {
		log.Fatalf("Failed to setup listener: %v", err)
	}
	defer l.Close()
	go handleCleanup(l)

	// Start handling incoming connections
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorf("Failed to accept connection: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

// Logic for socket cleanup on TERM/INT signal. On
// SIGKILL and other abrupt interruptions, the socket
// file is likely not cleaned up, and has to be deleted
// manually. TODO: Detect & fix this state in the CLI.
func handleCleanup(l net.Listener) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	log.Infof("Received signal: %s. Cleaning up...", <-sig)

	l.Close()

	mutex.Lock()
	for _, t := range tunnels {
		t.Close()
	}

	os.Exit(0)
}

func initLogger(path string) {
	logFile, err := os.OpenFile(path,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
}

func setupListener() (net.Listener, error) {
	return net.Listen("unix", daemon.SOCK)
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Receive command
	var cmd daemon.Command
	if err := ipc.Receive(&cmd, conn); err != nil {
		log.Errorf("Could not receive command: %v", err)
		return
	}

	// Execute command
	err := error(nil)
	switch cmd.Kind {
	case daemon.Open:
		err = openTunnel(cmd.Tunnel)
	case daemon.Close:
		err = closeTunnel(cmd.Tunnel)
	default:
		err = fmt.Errorf("unknown command: %v", cmd.Kind)
	}

	// Serialize & send response
	resp := daemon.Response{Success: true, Error: ""}
	if err != nil {
		resp = daemon.Response{Success: false, Error: err.Error()}
	}

	if err = ipc.Send(resp, conn); err != nil {
		log.Errorf("could not send response: %v", err)
	}
}

func openTunnel(t tunnel.Tunnel) error {
	if err := t.Open(); err != nil {
		return fmt.Errorf("could not start tunnel %v: %v", t.Name, err)
	}

	mutex.Lock()
	tunnels[t.Name] = &t
	mutex.Unlock()

	// Register reconnection logic
	go func() {
		<-t.Disconnected
		log.Infof("Detected disconnection of tunnel %v", t.Name)
		// Handle reconnection or other logic here
		// TODO
	}()

	return nil
}

func closeTunnel(q tunnel.Tunnel) error {
	// Lookup t in local tunnels map
	mutex.RLock()
	t, ok := tunnels[q.Name]
	mutex.RUnlock()
	if !ok {
		return fmt.Errorf("tunnel not running")
	}

	if err := t.Close(); err != nil {
		return fmt.Errorf("could not close tunnel: %v", err)
	}
	mutex.Lock()
	delete(tunnels, t.Name)
	mutex.Unlock()

	return nil
}

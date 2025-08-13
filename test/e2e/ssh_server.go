package e2e

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

const (
	loopBack          = "127.0.0.1:58391"
	hostKeyFile       = "../testdata/keys/server"
	authorizedKeyFile = "../testdata/keys/client.pub"
)

type tcpipForwardRequest struct {
	Addr string
	Port uint32
}

type forwardedTCPPayload struct {
	Addr       string
	Port       uint32
	OriginAddr string
	OriginPort uint32
}

func loadHostKey(path string) (ssh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(data)
}

func loadAuthorizedKey(path string) (ssh.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey(data)
	return key, err
}

func keysEqual(a, b ssh.PublicKey) bool {
	return a.Type() == b.Type() && string(a.Marshal()) == string(b.Marshal())
}

// Simple mock of an SSH server for testing
type sshServer struct {
	config   *ssh.ServerConfig
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}

	// these allow temporary pausing of connections
	pauseMu   sync.Mutex
	paused    bool
	pauseCond *sync.Cond

	// these record received keep-alives
	keepAliveMu sync.Mutex
	keepAlives  int
}

func startServer() (s *sshServer, err error) {
	authorized, err := loadAuthorizedKey(authorizedKeyFile)
	if err != nil {
		return nil, err
	}

	s = &sshServer{}
	s.config = &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if keysEqual(key, authorized) {
				return nil, nil
			}
			return nil, fmt.Errorf("unauthorized")
		},
	}

	s.conns = make(map[net.Conn]struct{})

	s.pauseCond = sync.NewCond(&s.pauseMu)

	hostKey, err := loadHostKey(hostKeyFile)
	if err != nil {
		return nil, err
	}
	s.config.AddHostKey(hostKey)

	s.listener, err = net.Listen("tcp", loopBack)
	if err != nil {
		return nil, fmt.Errorf("failed to listen for connection: %v", err)
	}

	go func() {
		for {
			s.pauseMu.Lock()
			for s.paused {
				s.pauseCond.Wait()
			}
			s.pauseMu.Unlock()

			conn, err := s.listener.Accept()
			if err != nil {
				if s.paused {
					continue // listener was paused, skip this accept
				}
				return
			}
			go s.handleConn(conn)
		}
	}()

	return s, nil
}

func (s *sshServer) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	c, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}

	go func() {
		for req := range reqs {
			if req.Type == "tcpip-forward" {
				// parse payload, reply true
				var payload tcpipForwardRequest
				if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
					req.Reply(false, nil)
					return
				}
				req.Reply(true, nil)
				go listenAndForward(c, payload)
			} else {
				if req.Type == "keepalive@golang.org" {
					s.incrementKeepAlives()
				}
				req.Reply(false, nil)
			}
		}
	}()

	for newChannel := range chans {
		if newChannel.ChannelType() == "direct-tcpip" {
			channel, requests, err := newChannel.Accept()
			if err != nil {
				return
			}
			go ssh.DiscardRequests(requests)
			go handleForwardedConnection(channel, newChannel.ExtraData())
		} else {
			newChannel.Reject(ssh.UnknownChannelType, "no channels supported")
		}
	}
}

func listenAndForward(c *ssh.ServerConn, req tcpipForwardRequest) {
	remote := c.RemoteAddr().(*net.TCPAddr)
	payload := ssh.Marshal(forwardedTCPPayload{
		Addr:       req.Addr,
		Port:       req.Port,
		OriginAddr: remote.IP.String(),
		OriginPort: uint32(remote.Port),
	})

	l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", req.Addr, req.Port))
	if err != nil {
		fmt.Errorf("failed to listen on %s:%d: %v\n", req.Addr, req.Port, err)
		return
	}
	defer l.Close()

	// Close the listener when the server connection is closed
	go func() {
		c.Wait()
		l.Close()
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Errorf("failed to accept connection: %v\n", err)
			return
		}
		go func() {
			defer conn.Close()
			ch, reqs, err := c.OpenChannel("forwarded-tcpip", payload)
			if err != nil {
				return
			}
			defer ch.Close()
			go ssh.DiscardRequests(reqs)
			go io.Copy(ch, conn)
			io.Copy(conn, ch)
		}()
	}
}

func handleForwardedConnection(channel ssh.Channel, extra []byte) {
	defer channel.Close()

	var payload forwardedTCPPayload
	if err := ssh.Unmarshal(extra, &payload); err != nil {
		fmt.Errorf("failed to unmarshal forwarded-tcpip payload: %v\n", err)
		return
	}
	addr := fmt.Sprintf("%s:%d", payload.Addr, payload.Port)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Errorf("failed to connect to %s: %v\n", addr, err)
		return
	}
	defer conn.Close()
	go io.Copy(conn, channel)
	io.Copy(channel, conn)
}

func (s *sshServer) cleanup() {
	s.listener.Close()
}

func (s *sshServer) pause() {
	s.pauseMu.Lock()
	s.paused = true
	// cancel current listening attempt
	//_ = s.listener.(*net.TCPListener).SetDeadline(time.Now())
	s.listener.Close()
	s.pauseMu.Unlock()
}

func (s *sshServer) resume() {
	s.pauseMu.Lock()
	s.paused = false
	// reset the deadline to allow new connections
	//_ = s.listener.(*net.TCPListener).SetDeadline(time.Time{})
	var err error
	s.listener, err = net.Listen("tcp", loopBack)
	if err != nil {
		panic("failed to listen for connection")
	}
	time.Sleep(20 * time.Millisecond) // give some time for the listener to be ready
	s.pauseMu.Unlock()
	s.pauseCond.Broadcast()
}

func (s *sshServer) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn := range s.conns {
		conn.Close()
		delete(s.conns, conn)
	}
}

func (s *sshServer) resetKeepAlives() {
	s.keepAliveMu.Lock()
	defer s.keepAliveMu.Unlock()
	s.keepAlives = 0
}

func (s *sshServer) incrementKeepAlives() {
	s.keepAliveMu.Lock()
	defer s.keepAliveMu.Unlock()
	s.keepAlives += 1
}

// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/alebeck/boring/internal/log"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	xproxy "golang.org/x/net/proxy"
)

func socks5Server(listener net.Listener) {
	var server Server
	err := server.Serve(listener)
	if err != nil {
		panic(err)
	}
	listener.Close()
}

func backendServer(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		panic(err)
	}
	conn.Write([]byte("Test"))
	conn.Close()
	listener.Close()
}

func udpEchoServer(conn net.PacketConn) {
	var buf [1024]byte
	n, addr, err := conn.ReadFrom(buf[:])
	if err != nil {
		panic(err)
	}
	_, err = conn.WriteTo(buf[:n], addr)
	if err != nil {
		panic(err)
	}
	conn.Close()
}

func TestMain(m *testing.M) {
	log.Init(os.Stdout, true, false)
	os.Exit(m.Run())
}

func TestRead(t *testing.T) {
	// backend server which we'll use SOCKS5 to connect to
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	backendServerPort := listener.Addr().(*net.TCPAddr).Port
	go backendServer(listener)

	// SOCKS5 server
	socks5, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	socks5Port := socks5.Addr().(*net.TCPAddr).Port
	go socks5Server(socks5)

	addr := fmt.Sprintf("localhost:%d", socks5Port)
	socksDialer, err := xproxy.SOCKS5("tcp", addr, nil, xproxy.Direct)
	if err != nil {
		t.Fatal(err)
	}

	addr = fmt.Sprintf("localhost:%d", backendServerPort)
	conn, err := socksDialer.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != "Test" {
		t.Fatalf("got: %q want: Test", buf)
	}

	err = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadPassword(t *testing.T) {
	// backend server which we'll use SOCKS5 to connect to
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	backendServerPort := ln.Addr().(*net.TCPAddr).Port
	go backendServer(ln)

	socks5ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		socks5ln.Close()
	})
	auth := &xproxy.Auth{User: "foo", Password: "bar"}
	go func() {
		s := Server{Username: auth.User, Password: auth.Password}
		err := s.Serve(socks5ln)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			panic(err)
		}
	}()

	expectDialErr := func(addr string, auth *xproxy.Auth) {
		if d, err := xproxy.SOCKS5("tcp", addr, auth, xproxy.Direct); err != nil {
			t.Fatal(err)
		} else {
			if _, err := d.Dial("tcp", addr); err == nil {
				t.Fatal("expected dial error")
			}
		}
	}

	addr := fmt.Sprintf("localhost:%d", socks5ln.Addr().(*net.TCPAddr).Port)
	expectDialErr(addr, nil)

	badPwd := &xproxy.Auth{User: "foo", Password: "not right"}
	expectDialErr(addr, badPwd)

	badUsr := &xproxy.Auth{User: "not right", Password: "bar"}
	expectDialErr(addr, badUsr)

	socksDialer, err := xproxy.SOCKS5("tcp", addr, auth, xproxy.Direct)
	if err != nil {
		t.Fatal(err)
	}

	addr = fmt.Sprintf("localhost:%d", backendServerPort)
	conn, err := socksDialer.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "Test" {
		t.Fatalf("got: %q want: Test", buf)
	}

	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func newUdpAssociateConn(t *testing.T, port int) (socks5Conn net.Conn, socks5UDPAddr socksAddr) {
	// net/proxy don't support UDP, so we need to manually send the SOCKS5 UDP request
	conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Write([]byte{socks5Version, 0x01, noAuthRequired}) // client hello with no auth
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf) // server hello
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || buf[0] != socks5Version || buf[1] != noAuthRequired {
		t.Fatalf("got: %q want: 0x05 0x00", buf[:n])
	}

	targetAddr := socksAddr{addrType: ipv4, addr: "0.0.0.0", port: 0}
	targetAddrPkt, err := targetAddr.marshal()
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Write(append([]byte{socks5Version, byte(udpAssociate), 0x00}, targetAddrPkt...)) // client reqeust
	if err != nil {
		t.Fatal(err)
	}

	n, err = conn.Read(buf) // server response
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 || !bytes.Equal(buf[:3], []byte{socks5Version, 0x00, 0x00}) {
		t.Fatalf("got: %q want: 0x05 0x00 0x00", buf[:n])
	}
	udpProxySocksAddr, err := parseSocksAddr(bytes.NewReader(buf[3:n]))
	if err != nil {
		t.Fatal(err)
	}

	return conn, udpProxySocksAddr
}

func TestUDP(t *testing.T) {
	// backend UDP server which we'll use SOCKS5 to connect to
	newUDPEchoServer := func() net.PacketConn {
		listener, err := net.ListenPacket("udp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		go udpEchoServer(listener)
		return listener
	}

	const echoServerNumber = 3
	echoServerListener := make([]net.PacketConn, echoServerNumber)
	for i := 0; i < echoServerNumber; i++ {
		echoServerListener[i] = newUDPEchoServer()
	}
	defer func() {
		for i := 0; i < echoServerNumber; i++ {
			_ = echoServerListener[i].Close()
		}
	}()

	// SOCKS5 server
	socks5, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	socks5Port := socks5.Addr().(*net.TCPAddr).Port
	go socks5Server(socks5)

	// make a socks5 udpAssociate conn
	conn, udpProxySocksAddr := newUdpAssociateConn(t, socks5Port)
	defer conn.Close()

	sendUDPAndWaitResponse := func(socks5UDPConn net.Conn, addr socksAddr, body []byte) (responseBody []byte) {
		udpPayload, err := (&udpRequest{addr: addr}).marshal()
		if err != nil {
			t.Fatal(err)
		}
		udpPayload = append(udpPayload, body...)
		_, err = socks5UDPConn.Write(udpPayload)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1024)
		n, err := socks5UDPConn.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		_, responseBody, err = parseUDPRequest(buf[:n])
		if err != nil {
			t.Fatal(err)
		}
		return responseBody
	}

	udpProxyAddr, err := net.ResolveUDPAddr("udp", udpProxySocksAddr.hostPort())
	if err != nil {
		t.Fatal(err)
	}
	socks5UDPConn, err := net.DialUDP("udp", nil, udpProxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer socks5UDPConn.Close()

	for i := 0; i < len(echoServerListener); i++ {
		port := echoServerListener[i].LocalAddr().(*net.UDPAddr).Port
		addr := socksAddr{addrType: ipv4, addr: "127.0.0.1", port: uint16(port)}
		requestBody := []byte(fmt.Sprintf("Test %d", i))
		responseBody := sendUDPAndWaitResponse(socks5UDPConn, addr, requestBody)
		if !bytes.Equal(requestBody, responseBody) {
			t.Fatalf("got: %q want: %q", responseBody, requestBody)
		}
	}
}

func dialSocks(t *testing.T, addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte{socks5Version, 0x01, noAuthRequired}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestParseClientGreetingErrors(t *testing.T) {
	cases := [][]byte{
		{socks5Version},
		{4, 1, noAuthRequired},
		{socks5Version, 2, noAuthRequired},
		{socks5Version, 1, passwordAuth},
	}
	for _, in := range cases {
		if err := parseClientGreeting(bytes.NewReader(in), noAuthRequired); err == nil {
			t.Errorf("input %v: expected error", in)
		}
	}
}

func TestParseClientAuthErrors(t *testing.T) {
	cases := [][]byte{
		{passwordAuthVersion},
		{2, 0},
		{passwordAuthVersion, 3, 'a'},
		{passwordAuthVersion, 1, 'a'},
		{passwordAuthVersion, 1, 'a', 2, 'x'},
	}
	for _, in := range cases {
		if _, _, err := parseClientAuth(bytes.NewReader(in)); err == nil {
			t.Errorf("input %v: expected error", in)
		}
	}
}

func TestParseClientRequestHeaderError(t *testing.T) {
	if _, err := parseClientRequest(bytes.NewReader([]byte{socks5Version, 1})); err == nil {
		t.Error("expected error")
	}
}

func TestParseSocksAddrErrors(t *testing.T) {
	cases := [][]byte{
		{},
		{byte(ipv4), 1, 2},
		{byte(domainName)},
		{byte(domainName), 5, 'a'},
		{byte(ipv6), 1, 2},
		{99},
		{byte(ipv4), 1, 2, 3, 4},
	}
	for _, in := range cases {
		if _, err := parseSocksAddr(bytes.NewReader(in)); err == nil {
			t.Errorf("input %v: expected error", in)
		}
	}
}

func TestSocksAddrMarshalErrors(t *testing.T) {
	cases := []socksAddr{
		{addrType: ipv4, addr: "not-an-ip"},
		{addrType: domainName, addr: strings.Repeat("a", 256)},
		{addrType: ipv6, addr: "not-an-ip"},
		{addrType: 99, addr: "x"},
	}
	for _, a := range cases {
		if _, err := a.marshal(); err == nil {
			t.Errorf("addr %v: expected error", a)
		}
	}
}

func TestSplitHostPortErrors(t *testing.T) {
	for _, in := range []string{"noport", "host:abc", "host:99999"} {
		if _, _, err := splitHostPort(in); err == nil {
			t.Errorf("input %q: expected error", in)
		}
	}
}

func TestParseUDPRequestErrors(t *testing.T) {
	cases := [][]byte{
		{0, 0, 0},
		{0, 1, 0, byte(ipv4)},
	}
	for _, in := range cases {
		if _, _, err := parseUDPRequest(in); err == nil {
			t.Errorf("input %v: expected error", in)
		}
	}
}

func TestGetAddrType(t *testing.T) {
	if getAddrType("example.com") != domainName {
		t.Error("want domainName")
	}
	if getAddrType("::1") != ipv6 {
		t.Error("want ipv6")
	}
	if getAddrType("1.2.3.4") != ipv4 {
		t.Error("want ipv4")
	}
}

func TestResponseMarshal(t *testing.T) {
	if errorResponse(generalFailure).reply != generalFailure {
		t.Error("wrong reply code")
	}
	bad := &response{reply: success, bindAddr: socksAddr{addrType: ipv4, addr: "bad"}}
	if _, err := bad.marshal(); err == nil {
		t.Error("expected marshal error")
	}
}

func TestUDPRequestMarshalError(t *testing.T) {
	if _, err := (&udpRequest{addr: socksAddr{addrType: 99}}).marshal(); err == nil {
		t.Error("expected error")
	}
}

func TestIsTimeout(t *testing.T) {
	if !isTimeout(fmt.Errorf("read: %w", os.ErrDeadlineExceeded)) {
		t.Error("want timeout true")
	}
	if isTimeout(errors.New("plain")) {
		t.Error("want timeout false")
	}
}

func TestUnsupportedCommand(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go socks5Server(ln)
	conn := dialSocks(t, fmt.Sprintf("localhost:%d", port))
	defer conn.Close()

	addrPkt, err := zeroSocksAddr.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append([]byte{socks5Version, byte(bind), 0x00}, addrPkt...)); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 16)
	n, err := conn.Read(resp)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 || resp[1] != byte(commandNotSupported) {
		t.Fatalf("got %v, want command not supported", resp[:n])
	}
}

func TestHandleTCPDialError(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		s := Server{Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		}}
		if err := s.Serve(ln); err != nil && !errors.Is(err, net.ErrClosed) {
			panic(err)
		}
	}()

	conn := dialSocks(t, fmt.Sprintf("localhost:%d", port))
	defer conn.Close()

	addrPkt, err := (socksAddr{addrType: ipv4, addr: "127.0.0.1", port: 9}).marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(append([]byte{socks5Version, byte(connect), 0x00}, addrPkt...)); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 16)
	n, err := conn.Read(resp)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 || resp[1] != byte(generalFailure) {
		t.Fatalf("got %v, want general failure", resp[:n])
	}
}

func TestHandleRequestParseError(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go socks5Server(ln)
	conn := dialSocks(t, fmt.Sprintf("localhost:%d", port))
	defer conn.Close()

	if _, err := conn.Write([]byte{socks5Version}); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 16)
	n, err := conn.Read(resp)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 || resp[1] != byte(generalFailure) {
		t.Fatalf("got %v, want general failure", resp[:n])
	}
}

// SPDX-License-Identifier: MIT

// Package socks5test is a scripted SOCKS5 proxy (RFC 1928, CONNECT only) for
// tests: it records each CONNECT and forwards it to an address the test picks.
package socks5test

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
)

// Request is one CONNECT the proxy received.
type Request struct {
	AddrType byte   // 1 IPv4, 3 domain name, 4 IPv6
	Host     string // the IP or the name as sent
	Port     int
}

// Server is a SOCKS5 proxy on loopback.
type Server struct {
	// Addr is the proxy's host:port.
	Addr string
	// Forward returns where to connect for a requested host:port; nil
	// connects to the requested address itself.
	Forward func(hostPort string) string
	// Reply is the CONNECT reply code; 0 succeeds.
	Reply byte
	// Method is the selected authentication method; 0 is "none", 0xFF
	// refuses every method.
	Method byte
	// Silent reads the greeting and never answers.
	Silent bool
	// Version overrides the version octet of the method reply (default 5).
	Version byte
	// BoundType is the address type of the bound address in a successful
	// reply: 1 (default), 3 or 4; any other value is sent as is.
	BoundType byte

	ln   net.Listener
	wg   sync.WaitGroup
	mu   sync.Mutex
	reqs []Request
}

// listen is net.Listen; a test replaces it to make Start fail.
var listen = net.Listen

// Start listens on loopback and serves until the test ends. Set the
// scripting fields of the returned Server before the first connection.
func Start(t testing.TB) *Server {
	t.Helper()
	ln, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
		return nil // only reached with a testing.TB whose Fatalf returns
	}
	s := &Server{Addr: ln.Addr().String(), ln: ln}
	s.wg.Go(s.accept)
	t.Cleanup(func() {
		_ = ln.Close()
		s.wg.Wait()
	})
	return s
}

// URL returns "socks5://<Addr>".
func (s *Server) URL() string { return "socks5://" + s.Addr }

// Requests returns the CONNECTs received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.reqs...)
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Go(func() { s.serve(c) })
	}
}

func (s *Server) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	var greet [2]byte
	if _, err := io.ReadFull(c, greet[:]); err != nil {
		return
	}
	if _, err := io.ReadFull(c, make([]byte, greet[1])); err != nil {
		return
	}
	if s.Silent {
		_, _ = io.Copy(io.Discard, c)
		return
	}
	version := s.Version
	if version == 0 {
		version = 5
	}
	if _, err := c.Write([]byte{version, s.Method}); err != nil || s.Method != 0 || version != 5 {
		return
	}
	var hdr [4]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return
	}
	var host string
	switch hdr[3] {
	case 1, 4:
		b := make([]byte, map[byte]int{1: 4, 4: 16}[hdr[3]])
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		host = string(b)
	default:
		return
	}
	var pb [2]byte
	if _, err := io.ReadFull(c, pb[:]); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(pb[:]))
	s.mu.Lock()
	s.reqs = append(s.reqs, Request{AddrType: hdr[3], Host: host, Port: port})
	s.mu.Unlock()

	if s.Reply != 0 {
		_, _ = c.Write([]byte{5, s.Reply, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	if s.Forward != nil {
		target = s.Forward(target)
	}
	up, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = up.Close() }()
	reply := []byte{5, 0, 0}
	switch s.BoundType {
	case 0, 1:
		reply = append(reply, 1, 127, 0, 0, 1)
	case 3:
		reply = append(reply, 3, 9)
		reply = append(reply, "localhost"...)
	case 4:
		reply = append(reply, 4)
		reply = append(reply, net.IPv6loopback...)
	default:
		reply = append(reply, s.BoundType)
	}
	reply = append(reply, 0, 0)
	// A failed write leaves c broken, so the copies below end at once.
	_, _ = c.Write(reply)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, c); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, up); done <- struct{}{} }()
	<-done
}

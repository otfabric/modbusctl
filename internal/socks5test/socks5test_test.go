// SPDX-License-Identifier: MIT

package socks5test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// client speaks the client side by hand and returns the reply octets.
func client(t *testing.T, s *Server, msgs ...[]byte) []byte {
	t.Helper()
	c, err := net.Dial("tcp", s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	for _, m := range msgs {
		if _, err := c.Write(m); err != nil {
			break
		}
	}
	// Half-close, so the proxy's copy and the echo end without a timeout.
	_ = c.(*net.TCPConn).CloseWrite()
	out, _ := io.ReadAll(c)
	return out
}

func echoAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	return ln.Addr().String()
}

var greeting = []byte{5, 1, 0}

func connect(atyp byte, addr ...byte) []byte {
	return append(append([]byte{5, 1, 0, atyp}, addr...), 0, 102)
}

func TestServer(t *testing.T) {
	target := echoAddr(t)
	ipv4 := connect(1, 127, 0, 0, 1)
	for _, tc := range []struct {
		name   string
		setup  func(*Server)
		msgs   [][]byte
		prefix []byte
		req    *Request
	}{
		{"ipv4, bound ipv4", nil, [][]byte{greeting, ipv4, []byte("hi")}, []byte{5, 0, 5, 0, 0, 1, 127, 0, 0, 1, 0, 0}, &Request{1, "127.0.0.1", 102}},
		{"name, bound name", func(s *Server) { s.BoundType = 3 }, [][]byte{greeting, connect(3, append([]byte{9}, "localhost"...)...), []byte("hi")}, []byte{5, 0, 5, 0, 0, 3, 9}, &Request{3, "localhost", 102}},
		{"ipv6, bound ipv6", func(s *Server) { s.BoundType = 4 }, [][]byte{greeting, connect(4, net.IPv6loopback...), []byte("hi")}, []byte{5, 0, 5, 0, 0, 4}, &Request{4, "::1", 102}},
		{"bound type raw", func(s *Server) { s.BoundType = 9 }, [][]byte{greeting, ipv4}, []byte{5, 0, 5, 0, 0, 9}, nil},
		{"reply code", func(s *Server) { s.Reply = 2 }, [][]byte{greeting, ipv4}, []byte{5, 0, 5, 2}, nil},
		{"auth refused", func(s *Server) { s.Method = 0xFF }, [][]byte{greeting}, []byte{5, 0xFF}, nil},
		{"version", func(s *Server) { s.Version = 4 }, [][]byte{greeting}, []byte{4, 0}, nil},
		{"silent", func(s *Server) { s.Silent = true }, [][]byte{greeting}, nil, nil},
		{"forward fails", func(s *Server) { s.Forward = func(string) string { return "127.0.0.1:1" } }, [][]byte{greeting, ipv4}, []byte{5, 0, 5, 5}, nil},
		{"unknown address type", nil, [][]byte{greeting, {5, 1, 0, 7}}, []byte{5, 0}, nil},
		{"cut greeting", nil, [][]byte{{5}}, nil, nil},
		{"cut methods", nil, [][]byte{{5, 2, 0}}, nil, nil},
		{"cut request", nil, [][]byte{greeting, {5, 1}}, []byte{5, 0}, nil},
		{"cut ipv4", nil, [][]byte{greeting, {5, 1, 0, 1, 127}}, []byte{5, 0}, nil},
		{"cut name length", nil, [][]byte{greeting, {5, 1, 0, 3}}, []byte{5, 0}, nil},
		{"cut name", nil, [][]byte{greeting, {5, 1, 0, 3, 9, 'l'}}, []byte{5, 0}, nil},
		{"cut port", nil, [][]byte{greeting, {5, 1, 0, 1, 127, 0, 0, 1, 0}}, []byte{5, 0}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Start(t)
			s.Forward = func(string) string { return target }
			if tc.setup != nil {
				tc.setup(s)
			}
			got := client(t, s, tc.msgs...)
			if len(got) < len(tc.prefix) || string(got[:len(tc.prefix)]) != string(tc.prefix) {
				t.Errorf("reply % x, want prefix % x", got, tc.prefix)
			}
			if tc.req != nil {
				if r := s.Requests(); len(r) != 1 || r[0] != *tc.req {
					t.Errorf("requests %+v, want %+v", r, *tc.req)
				}
			}
		})
	}
	if u := Start(t); u.URL() != "socks5://"+u.Addr {
		t.Errorf("URL = %q", u.URL())
	}
}

// fatalTB records Fatalf instead of stopping the test.
type fatalTB struct {
	testing.TB
	fatal string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(format string, args ...any) { f.fatal = fmt.Sprintf(format, args...) }

func TestStartListenFailure(t *testing.T) {
	errListen := errors.New("no sockets")
	listen = func(string, string) (net.Listener, error) { return nil, errListen }
	t.Cleanup(func() { listen = net.Listen })

	tb := &fatalTB{}
	if s := Start(tb); s != nil {
		t.Fatalf("Start returned %v after a listen failure, want nil", s)
	}
	if tb.fatal != "listen: no sockets" {
		t.Errorf("Fatalf message = %q, want the listen error", tb.fatal)
	}
}

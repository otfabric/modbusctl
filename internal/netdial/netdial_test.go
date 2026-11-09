// SPDX-License-Identifier: MIT

package netdial

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/socks5test"
)

// echo listens on loopback and echoes what it reads; it returns host:port.
func echo(t *testing.T) string {
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
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	return ln.Addr().String()
}

func roundTrip(t *testing.T, c net.Conn) {
	t.Helper()
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func ctx2s(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestNewNothingSet(t *testing.T) {
	d, err := New("", "")
	if d != nil || err != nil {
		t.Fatalf("New(\"\", \"\") = %v, %v; want nil, nil", d != nil, err)
	}
}

func TestNewInvalid(t *testing.T) {
	for _, tc := range []struct{ src, proxy, want string }{
		{"not-an-ip", "", "--source-addr"},
		{"", "http://127.0.0.1:8080", "scheme must be socks5"},
		{"", "socks5://user:pw@127.0.0.1:1080", "credentials are not supported"},
		{"", "socks5://127.0.0.1", "want socks5://host:port"},
		{"", "socks5://:1080", "want socks5://host:port"},
		{"", "socks5://127.0.0.1:0", "invalid port"},
		{"", "socks5://127.0.0.1:99999", "invalid port"},
		{"", "socks5://127.0.0.1:1080/path", "want socks5://host:port"},
		{"", "socks5://127.0.0.1:1080?x=1", "want socks5://host:port"},
		{"", "socks5://[::1", "invalid --proxy"},
	} {
		if _, err := New(tc.src, tc.proxy); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("New(%q, %q) error = %v, want it to mention %q", tc.src, tc.proxy, err, tc.want)
		}
	}
}

func TestSourceAddrBinds(t *testing.T) {
	d, err := New("127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := d(ctx2s(t), "tcp", echo(t))
	if err != nil {
		t.Fatal(err)
	}
	if ip := c.LocalAddr().(*net.TCPAddr).IP; !ip.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("local address %v, want 127.0.0.1", ip)
	}
	roundTrip(t, c)
}

func TestProxyConnect(t *testing.T) {
	target := echo(t)
	host, portStr, _ := net.SplitHostPort(target)
	port, _ := strconv.Atoi(portStr)
	for _, bound := range []byte{1, 3, 4} {
		px := socks5test.Start(t)
		px.BoundType = bound
		d, err := New("127.0.0.1", px.URL())
		if err != nil {
			t.Fatal(err)
		}
		c, err := d(ctx2s(t), "tcp", target)
		if err != nil {
			t.Fatalf("bound type %d: %v", bound, err)
		}
		roundTrip(t, c)
		if r := px.Requests(); len(r) != 1 || r[0].AddrType != 1 || r[0].Host != host || r[0].Port != port {
			t.Errorf("bound type %d: proxy saw %+v, want IPv4 %s:%d", bound, r, host, port)
		}
	}
}

func TestProxyIPv6Target(t *testing.T) {
	px := socks5test.Start(t)
	target := echo(t)
	px.Forward = func(string) string { return target }
	d, _ := New("", px.URL())
	c, err := d(ctx2s(t), "tcp", "[2001:db8::7]:102")
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c)
	if r := px.Requests(); len(r) != 1 || r[0].AddrType != 4 || r[0].Host != "2001:db8::7" || r[0].Port != 102 {
		t.Errorf("proxy saw %+v, want IPv6 2001:db8::7:102", r)
	}
}

func TestProxyHostNames(t *testing.T) {
	target := echo(t)
	// socks5h sends the name; socks5 resolves it here first.
	for _, tc := range []struct {
		scheme   string
		addrType byte
	}{{"socks5h", 3}, {"socks5", 0}} {
		px := socks5test.Start(t)
		px.Forward = func(string) string { return target }
		d, _ := New("", tc.scheme+"://"+px.Addr)
		c, err := d(ctx2s(t), "tcp", "localhost:102")
		if err != nil {
			t.Fatalf("%s: %v", tc.scheme, err)
		}
		roundTrip(t, c)
		r := px.Requests()
		switch {
		case len(r) != 1:
			t.Errorf("%s: %d requests", tc.scheme, len(r))
		case tc.addrType == 3 && (r[0].AddrType != 3 || r[0].Host != "localhost"):
			t.Errorf("%s: proxy saw %+v, want the name", tc.scheme, r[0])
		case tc.addrType == 0 && r[0].AddrType == 3:
			t.Errorf("%s: proxy saw the name %+v, want a resolved IP", tc.scheme, r[0])
		}
	}
}

func TestProxyRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*socks5test.Server)
		check func(error) bool
	}{
		{"connection refused", func(s *socks5test.Server) { s.Reply = 5 },
			func(err error) bool {
				var re *ReplyError
				return errors.As(err, &re) && re.Code == 5 && strings.Contains(err.Error(), "connection refused")
			}},
		{"ruleset", func(s *socks5test.Server) { s.Reply = 2 },
			func(err error) bool { return strings.Contains(err.Error(), "not allowed by ruleset") }},
		{"unknown code", func(s *socks5test.Server) { s.Reply = 0x42 },
			func(err error) bool { return strings.Contains(err.Error(), "reply code 66") }},
		{"authentication required", func(s *socks5test.Server) { s.Method = 0xFF },
			func(err error) bool { return strings.Contains(err.Error(), "requires authentication") }},
		{"wrong version", func(s *socks5test.Server) { s.Version = 4 },
			func(err error) bool { return errors.Is(err, errProtocol) }},
		{"bad bound address type", func(s *socks5test.Server) { s.BoundType = 9 },
			func(err error) bool { return errors.Is(err, errProtocol) }},
		{"silent proxy times out", func(s *socks5test.Server) { s.Silent = true },
			func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			px := socks5test.Start(t)
			tc.setup(px)
			d, _ := New("", px.URL())
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			c, err := d(ctx, "tcp", echo(t))
			if c != nil || err == nil || !tc.check(err) {
				t.Fatalf("dial = %v, %v", c, err)
			}
			if !strings.Contains(err.Error(), "socks5 proxy "+px.Addr) {
				t.Errorf("error %q does not name the proxy", err)
			}
		})
	}
}

func TestProxyCancel(t *testing.T) {
	px := socks5test.Start(t)
	px.Silent = true
	d, _ := New("", px.URL())
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := d(ctx, "tcp", "127.0.0.1:102"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestProxyErrors(t *testing.T) {
	d, _ := New("", "socks5://127.0.0.1:1")
	if _, err := d(ctx2s(t), "udp", "127.0.0.1:102"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("udp: %v", err)
	}
	if _, err := d(ctx2s(t), "tcp", "no-port"); err == nil {
		t.Error("address without port: no error")
	}
	if _, err := d(ctx2s(t), "tcp", "127.0.0.1:70000"); err == nil || !strings.Contains(err.Error(), "invalid port") {
		t.Errorf("bad port: %v", err)
	}
	long := strings.Repeat("a", 256) + ":102"
	dh, _ := New("", "socks5h://127.0.0.1:1")
	if _, err := dh(ctx2s(t), "tcp", long); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("long name: %v", err)
	}
	if _, err := d(ctx2s(t), "tcp", "name.invalid:102"); err == nil {
		t.Error("unresolvable name with socks5: no error")
	}
	// Nothing listens on the proxy port.
	if _, err := d(ctx2s(t), "tcp", "127.0.0.1:102"); err == nil || !strings.Contains(err.Error(), "socks5 proxy 127.0.0.1:1") {
		t.Errorf("proxy down: %v", err)
	}
}

// handshake against a scripted peer on net.Pipe: each step's failure.
func TestHandshakeFailures(t *testing.T) {
	req := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 102}
	for _, tc := range []struct {
		name string
		peer func(c net.Conn)
	}{
		{"peer gone before greeting", func(c net.Conn) { _ = c.Close() }},
		{"peer gone before request", func(c net.Conn) {
			_, _ = io.ReadFull(c, make([]byte, 3))
			_, _ = c.Write([]byte{5, 0})
			_ = c.Close()
		}},
		{"peer gone before reply", func(c net.Conn) {
			_, _ = io.ReadFull(c, make([]byte, 3))
			_, _ = c.Write([]byte{5, 0})
			_, _ = io.ReadFull(c, make([]byte, len(req)))
			_ = c.Close()
		}},
		{"reply with wrong version", func(c net.Conn) {
			_, _ = io.ReadFull(c, make([]byte, 3))
			_, _ = c.Write([]byte{5, 0})
			_, _ = io.ReadFull(c, make([]byte, len(req)))
			_, _ = c.Write([]byte{4, 0, 0, 1})
		}},
		{"name bound address cut", func(c net.Conn) {
			_, _ = io.ReadFull(c, make([]byte, 3))
			_, _ = c.Write([]byte{5, 0})
			_, _ = io.ReadFull(c, make([]byte, len(req)))
			_, _ = c.Write([]byte{5, 0, 0, 3})
			_ = c.Close()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer func() { _ = a.Close() }()
			go func() { defer func() { _ = b.Close() }(); tc.peer(b) }()
			_ = a.SetDeadline(time.Now().Add(2 * time.Second))
			if err := handshake(a, req); err == nil {
				t.Fatal("no error")
			}
		})
	}
}

func TestSourceAddrBindsUDP(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	d, err := New("127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := d(ctx2s(t), "udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	la, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || !la.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("local address %v, want 127.0.0.1 over udp", c.LocalAddr())
	}
	if _, err := c.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, from, err := pc.ReadFrom(make([]byte, 4))
	if err != nil || from.String() != la.String() {
		t.Fatalf("datagram from %v, %v; want %v", from, err, la)
	}
}

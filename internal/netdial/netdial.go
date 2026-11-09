// SPDX-License-Identifier: MIT

// Package netdial builds the modbus.Config.Dialer of the --source-addr and
// --proxy flags: a TCP or UDP dial bound to a local address, or a TCP dial
// through a SOCKS5 proxy (RFC 1928 CONNECT, no authentication).
package netdial

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"
)

// Dialer opens a connection to addr over network ("tcp" or "udp"), as
// net.Dialer.DialContext does; it is assignable to modbus.Dialer.
type Dialer = func(ctx context.Context, network, addr string) (net.Conn, error)

// New returns the dialer for sourceAddr (an IP literal to bind, or "") and
// proxy ("socks5://host:port", "socks5h://host:port" or ""). It returns nil
// when both are empty, so the library keeps its default dialer. Errors are
// usage errors: the flags are invalid.
func New(sourceAddr, proxy string) (Dialer, error) {
	if sourceAddr == "" && proxy == "" {
		return nil, nil
	}
	var base local
	if sourceAddr != "" {
		base.ip = net.ParseIP(sourceAddr)
		if base.ip == nil {
			return nil, fmt.Errorf("invalid --source-addr %q: want an IP address", sourceAddr)
		}
	}
	if proxy == "" {
		return base.dial, nil
	}
	p, err := parseProxy(proxy)
	if err != nil {
		return nil, err
	}
	p.base = base
	return p.dial, nil
}

// local dials directly, from ip when it is set: a TCP address for tcp
// networks, a UDP address for udp ones.
type local struct{ ip net.IP }

func (l local) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	if l.ip != nil {
		switch network {
		case "udp", "udp4", "udp6":
			d.LocalAddr = &net.UDPAddr{IP: l.ip}
		default:
			d.LocalAddr = &net.TCPAddr{IP: l.ip}
		}
	}
	return d.DialContext(ctx, network, addr)
}

type socks5 struct {
	addr       string // proxy host:port
	remoteName bool   // socks5h: the proxy resolves host names
	base       local
}

func parseProxy(s string) (*socks5, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid --proxy %q: %w", s, err)
	}
	if u.Scheme != "socks5" && u.Scheme != "socks5h" {
		return nil, fmt.Errorf("invalid --proxy %q: the scheme must be socks5 or socks5h", s)
	}
	if u.User != nil {
		return nil, fmt.Errorf("invalid --proxy %q: proxy credentials are not supported", s)
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid --proxy %q: want socks5://host:port", s)
	}
	host, port := u.Hostname(), u.Port()
	if host == "" || port == "" {
		return nil, fmt.Errorf("invalid --proxy %q: want socks5://host:port", s)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, fmt.Errorf("invalid --proxy %q: invalid port", s)
	}
	return &socks5{addr: net.JoinHostPort(host, port), remoteName: u.Scheme == "socks5h"}, nil
}

// SOCKS5 reply codes (RFC 1928 section 6).
var replyText = map[byte]string{
	1: "general SOCKS server failure",
	2: "connection not allowed by ruleset",
	3: "network unreachable",
	4: "host unreachable",
	5: "connection refused",
	6: "TTL expired",
	7: "command not supported",
	8: "address type not supported",
}

// ReplyError is a SOCKS5 proxy refusing the CONNECT.
type ReplyError struct{ Code byte }

func (e *ReplyError) Error() string {
	if t, ok := replyText[e.Code]; ok {
		return "socks5: " + t
	}
	return fmt.Sprintf("socks5: reply code %d", e.Code)
}

var errProtocol = errors.New("socks5: unexpected reply from the proxy")

func (p *socks5) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("socks5: network %q not supported", network)
	}
	req, err := p.connectRequest(ctx, addr)
	if err != nil {
		return nil, err
	}
	conn, err := p.base.dial(ctx, "tcp", p.addr)
	if err != nil {
		return nil, fmt.Errorf("socks5 proxy %s: %w", p.addr, err)
	}
	// The handshake honours ctx, its deadline and its cancel, through one
	// path: when ctx is done the pending I/O fails, so ctx.Err() is already
	// set and the error matches context.DeadlineExceeded / Canceled.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	err = handshake(conn, req)
	stop()
	if err == nil {
		// A ctx that ended as the handshake finished may have set the past
		// deadline already: fail rather than return a poisoned connection.
		err = ctx.Err()
	}
	if err != nil {
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
			err = fmt.Errorf("%w: %w", ctxErr, err)
		}
		return nil, fmt.Errorf("socks5 proxy %s: %w", p.addr, err)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// connectRequest encodes CONNECT for addr: an IP literal as such; a host
// name sent to the proxy (socks5h) or resolved here first (socks5).
func (p *socks5) connectRequest(ctx context.Context, addr string) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("socks5: invalid port in %q", addr)
	}
	req := []byte{5, 1, 0}
	ip := net.ParseIP(host)
	if ip == nil && !p.remoteName {
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		ip = ips[0].IP
	}
	switch {
	case ip == nil:
		if len(host) > 255 {
			return nil, fmt.Errorf("socks5: host name too long")
		}
		req = append(req, 3, byte(len(host)))
		req = append(req, host...)
	case ip.To4() != nil:
		req = append(req, 1)
		req = append(req, ip.To4()...)
	default:
		req = append(req, 4)
		req = append(req, ip.To16()...)
	}
	return binary.BigEndian.AppendUint16(req, uint16(port)), nil
}

func handshake(conn net.Conn, req []byte) error {
	// Greeting: version 5, one method, "no authentication".
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}
	var sel [2]byte
	if _, err := io.ReadFull(conn, sel[:]); err != nil {
		return err
	}
	if sel[0] != 5 {
		return errProtocol
	}
	if sel[1] != 0 {
		return errors.New("socks5: the proxy requires authentication, which is not supported")
	}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return err
	}
	if hdr[0] != 5 {
		return errProtocol
	}
	if hdr[1] != 0 {
		return &ReplyError{Code: hdr[1]}
	}
	var n int
	switch hdr[3] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(conn, l[:]); err != nil {
			return err
		}
		n = int(l[0])
	default:
		return errProtocol
	}
	// The bound address and port: read and discard.
	_, err := io.ReadFull(conn, make([]byte, n+2))
	return err
}

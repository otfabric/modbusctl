// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"runtime"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mdlayher/arp"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// arpTestConn stands in for the raw socket of an ARP client. It reads back every frame
// written to it, as a packet socket sees its own requests, and answers a request for an
// address of replies with a reply from that MAC. Then a read waits until the deadline
// passes or the conn is closed: no other host answers.
type arpTestConn struct {
	replies map[netip.Addr]net.HardwareAddr
	// expireAtOnce ends a read under a deadline at once, as if the deadline had passed.
	expireAtOnce bool

	mu        sync.Mutex
	deadline  time.Time
	frames    [][]byte
	writes    int
	closed    chan struct{}
	closeOnce sync.Once
	waiting   chan struct{} // closed when a read starts to wait
	waitOnce  sync.Once
}

func newARPTestConn() *arpTestConn {
	return &arpTestConn{closed: make(chan struct{}), waiting: make(chan struct{})}
}

func (c *arpTestConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *arpTestConn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.mu.Lock()
	if len(c.frames) > 0 {
		f := c.frames[0]
		c.frames = c.frames[1:]
		c.mu.Unlock()
		return copy(b, f), nil, nil
	}
	deadline := c.deadline
	c.mu.Unlock()
	if c.isClosed() {
		return 0, nil, net.ErrClosed
	}
	var expired <-chan time.Time
	if !deadline.IsZero() {
		if c.expireAtOnce {
			return 0, nil, os.ErrDeadlineExceeded
		}
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		expired = timer.C
	}
	c.waitOnce.Do(func() { close(c.waiting) })
	select {
	case <-c.closed:
		return 0, nil, net.ErrClosed
	case <-expired:
		return 0, nil, os.ErrDeadlineExceeded
	}
}

// arpFrame is an Ethernet frame (destination, source, EtherType 0x0806) carrying p.
func arpFrame(t testing.TB, dst, src net.HardwareAddr, p *arp.Packet) []byte {
	t.Helper()
	payload, err := p.MarshalBinary()
	require.NoError(t, err)
	f := append(append(append([]byte(nil), dst...), src...), 0x08, 0x06)
	return append(f, payload...)
}

func (c *arpTestConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	if c.isClosed() {
		return 0, net.ErrClosed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	c.frames = append(c.frames, append([]byte(nil), b...))
	// b = Ethernet header (14 bytes), then the ARP request: the sender's MAC at 8, its IPv4
	// address at 14 and the target address at 24.
	if len(b) >= 14+28 && binary.BigEndian.Uint16(b[12:14]) == 0x0806 {
		req := b[14:]
		target, _ := netip.AddrFromSlice(req[24:28])
		if mac, ok := c.replies[target]; ok {
			sender, _ := netip.AddrFromSlice(req[14:18])
			reply, err := arp.NewPacket(arp.OperationReply, mac, target, net.HardwareAddr(req[8:14]), sender)
			if err != nil {
				return 0, err
			}
			payload, err := reply.MarshalBinary()
			if err != nil {
				return 0, err
			}
			frame := append(append(append([]byte(nil), req[8:14]...), mac...), 0x08, 0x06)
			c.frames = append(c.frames, append(frame, payload...))
		}
	}
	return len(b), nil
}

func (c *arpTestConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *arpTestConn) LocalAddr() net.Addr { return nil }

func (c *arpTestConn) SetDeadline(t time.Time) error { return c.SetReadDeadline(t) }

func (c *arpTestConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	return nil
}

func (c *arpTestConn) SetWriteDeadline(time.Time) error { return nil }

// arpTestInterface returns a loopback interface with an IPv4 address: the ARP client takes
// its address as the sender of the requests.
func arpTestInterface(t *testing.T) *net.Interface {
	t.Helper()
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagLoopback == 0 {
			continue
		}
		addrs, err := ifaces[i].Addrs()
		require.NoError(t, err)
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				return &ifaces[i]
			}
		}
	}
	requireOrSkip(t, "no loopback interface with an IPv4 address")
	return nil
}

// useARPTestConn makes resolveMACAddress open its ARP client on conn, and counts the dials.
// The loopback interface has no hardware address, so the client gets one of its own.
func useARPTestConn(t *testing.T, conn *arpTestConn) *int {
	t.Helper()
	dials := new(int)
	old := dialARP
	dialARP = func(ifi *net.Interface) (*arp.Client, error) {
		*dials++
		withMAC := *ifi
		withMAC.HardwareAddr = net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}
		return arp.New(&withMAC, conn)
	}
	t.Cleanup(func() { dialARP = old })
	return dials
}

// resolveMACAddress waits for the reply at most discoveryMACTimeout, and stops waiting as soon
// as its context ends. The clock is the synctest bubble's.
func TestResolveMACAddress_bounded(t *testing.T) {
	iface := arpTestInterface(t)

	t.Run("no reply ends at the time bound", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			conn := newARPTestConn()
			dials := useARPTestConn(t, conn)
			start := time.Now()
			mac, err := resolveMACAddress(t.Context(), "192.0.2.10", iface.Name)
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Empty(t, mac)
			require.Equal(t, discoveryMACTimeout, time.Since(start))
			require.Equal(t, 1, *dials)
			require.Equal(t, 1, conn.writes, "one request is sent")
			require.True(t, conn.isClosed(), "the socket is closed")
		})
	})

	t.Run("the context ends while the reply is awaited", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			conn := newARPTestConn()
			useARPTestConn(t, conn)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			start := time.Now()
			done := make(chan error, 1)
			go func() {
				_, err := resolveMACAddress(ctx, "192.0.2.10", iface.Name)
				done <- err
			}()
			<-conn.waiting
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
			require.Zero(t, time.Since(start), "no time passes after the context ends")
			require.True(t, conn.isClosed())
		})
	})

	t.Run("an ended context sends nothing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			conn := newARPTestConn()
			dials := useARPTestConn(t, conn)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := resolveMACAddress(ctx, "192.0.2.10", iface.Name)
			require.ErrorIs(t, err, context.Canceled)
			require.Zero(t, *dials)
		})
	})

	t.Run("a reply within the bound gives the MAC", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			conn := newARPTestConn()
			want := net.HardwareAddr{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}
			conn.replies = map[netip.Addr]net.HardwareAddr{netip.MustParseAddr("192.0.2.10"): want}
			useARPTestConn(t, conn)
			mac, err := resolveMACAddress(t.Context(), "192.0.2.10", iface.Name)
			require.NoError(t, err)
			require.Equal(t, want.String(), mac)
		})
	})

	t.Run("replies for other hosts are skipped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			conn := newARPTestConn()
			other := net.HardwareAddr{0x02, 0x99, 0x99, 0x99, 0x99, 0x99}
			p, err := arp.NewPacket(arp.OperationReply, other, netip.MustParseAddr("192.0.2.99"),
				net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}, netip.MustParseAddr("127.0.0.1"))
			require.NoError(t, err)
			for range 3 {
				conn.frames = append(conn.frames, arpFrame(t, net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01}, other, p))
			}
			useARPTestConn(t, conn)
			start := time.Now()
			_, err = resolveMACAddress(t.Context(), "192.0.2.10", iface.Name)
			require.ErrorIs(t, err, os.ErrDeadlineExceeded)
			require.Equal(t, discoveryMACTimeout, time.Since(start))
		})
	})
}

// collDiscoverMAC runs CollectDiscover with --resolve-mac against a loopback device, with its
// ARP client on conn, and returns its result. When it does not return within 10 s, the test
// fails, and conn is closed so that the workers end before the test does.
func collDiscoverMAC(t *testing.T, ctx context.Context, conn *arpTestConn, iface string, port uint16) (*types.DiscoverOutput, error) {
	t.Helper()
	type result struct {
		out *types.DiscoverOutput
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := CollectDiscover(ctx, config.DiscoverConfig{
			Subnets: []string{collLoopback}, Port: port, Parallel: 2, ResolveMAC: true, NetworkInterface: iface,
		}, nil)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-time.After(10 * time.Second):
		_ = conn.Close()
		<-done
		t.Fatal("discover did not return while a MAC address was awaited")
		return nil, nil
	}
}

// A host that answers Modbus but not ARP is listed without a MAC once the bound has passed.
func TestCollDiscover_macWithoutReplyKeepsDevice(t *testing.T) {
	iface := arpTestInterface(t)
	conn := newARPTestConn()
	conn.expireAtOnce = true
	useARPTestConn(t, conn)
	dev := mbtest.NewDevice().SetHolding(0, 1)
	port := collPort(t, dev.ServeTCP(t))

	out, err := collDiscoverMAC(t, context.Background(), conn, iface.Name, port)
	require.NoError(t, err)
	require.Equal(t, []types.DiscoverJson{{IP: "127.0.0.1", Port: port, Interface: iface.Name}}, out.Devices)
	require.Equal(t, 1, conn.writes, "one ARP request is sent")
}

// Ctrl-C while a MAC address is awaited ends discover at once, with the hosts found so far.
func TestCollDiscover_cancelWhileMACAwaited(t *testing.T) {
	iface := arpTestInterface(t)
	conn := newARPTestConn()
	useARPTestConn(t, conn)
	dev := mbtest.NewDevice().SetHolding(0, 1)
	port := collPort(t, dev.ServeTCP(t))
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-conn.waiting
		cancel()
	}()

	out, err := collDiscoverMAC(t, ctx, conn, iface.Name, port)
	require.NoError(t, err, "an interrupted discover keeps its result (docs/exitcodes.md)")
	require.Equal(t, []types.DiscoverJson{{IP: "127.0.0.1", Port: port, Interface: iface.Name}}, out.Devices)
	require.True(t, conn.isClosed(), "the ARP socket is closed")
	requireGoroutinesBack(t, base)
}

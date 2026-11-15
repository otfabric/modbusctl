//go:build linux

// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The ARP path on a raw socket, on the interface MODBUSCTL_ARP_TEST_INTERFACE names: an up
// interface with an IPv4 address and a hardware address on which no host answers ARP, such
// as a dummy interface. It needs CAP_NET_RAW. Without the variable it is skipped, so that no
// test sends ARP requests on a network it did not choose.
func TestResolveMACAddress_rawSocket(t *testing.T) {
	name := os.Getenv("MODBUSCTL_ARP_TEST_INTERFACE")
	if name == "" {
		t.Skip("set MODBUSCTL_ARP_TEST_INTERFACE to an interface where no host answers ARP (needs CAP_NET_RAW)")
	}
	iface, err := net.InterfaceByName(name)
	require.NoError(t, err)
	addrs, err := iface.Addrs()
	require.NoError(t, err)
	var target string
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err == nil && p.Addr().Is4() {
			// The next address of the subnet: not the interface's own.
			target = p.Addr().Next().String()
			break
		}
	}
	require.NotEmpty(t, target, "%s has no IPv4 address", name)
	client, err := dialARP(iface)
	require.NoError(t, err, "a raw socket on %s (CAP_NET_RAW)", name)
	require.NoError(t, client.Close())

	t.Run("no reply ends at the time bound", func(t *testing.T) {
		start := time.Now()
		mac, err := resolveMACAddress(context.Background(), target, name)
		elapsed := time.Since(start)
		require.Error(t, err)
		require.Empty(t, mac)
		require.GreaterOrEqual(t, elapsed, discoveryMACTimeout-50*time.Millisecond)
		require.Less(t, elapsed, discoveryMACTimeout+time.Second)
	})

	t.Run("the context ends while the reply is awaited", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := resolveMACAddress(ctx, target, name)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(start), discoveryMACTimeout)
	})
}

// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

const collLoopback = "127.0.0.1/32"

func TestCollDiscover_findsDevice(t *testing.T) {
	tests := []struct {
		name string
		dev  *mbtest.Device
	}{
		{name: "register 0 readable", dev: mbtest.NewDevice().SetHolding(0, 1)},
		{name: "exception answer still proves a Modbus device", dev: mbtest.NewDevice()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			port := collPort(t, tc.dev.ServeTCP(t))
			out, err := CollectDiscover(context.Background(), config.DiscoverConfig{
				// The duplicate subnet must be probed once.
				Subnets: []string{collLoopback, collLoopback}, Port: port, Parallel: 4, NetworkInterface: "eth-test",
			}, nil)
			require.NoError(t, err)
			require.Equal(t, port, out.Port)
			require.Equal(t, "eth-test", out.Interface)
			require.Equal(t, []string{collLoopback, collLoopback}, out.Subnets)
			require.Equal(t, []types.DiscoverJson{{IP: "127.0.0.1", Port: port, Interface: "eth-test"}}, out.Devices)
			require.Equal(t, 1, tc.dev.RequestCount(0), "one probe per unique host")
			require.Equal(t, 1, tc.dev.RequestCount(0x03))
		})
	}
}

func TestCollDiscover_nothingFound(t *testing.T) {
	t.Run("closed port", func(t *testing.T) {
		port := collPort(t, collClosedAddr(t))
		var stderr collSyncBuffer
		// Parallel 0 is treated as one worker.
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Debug: true}, &stderr)
		require.NoError(t, err)
		require.NotNil(t, out.Devices, "JSON output needs [] rather than null")
		require.Empty(t, out.Devices)
		require.Contains(t, stderr.String(), "DEBUG [discover] 127.0.0.1: connect/setup:")
	})

	t.Run("closed port without debug stays quiet", func(t *testing.T) {
		port := collPort(t, collClosedAddr(t))
		var stderr collSyncBuffer
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Parallel: 1}, &stderr)
		require.NoError(t, err)
		require.Empty(t, out.Devices)
		require.Empty(t, stderr.String())
	})

	t.Run("listener that is not a Modbus device", func(t *testing.T) {
		dev := mbtest.NewDevice()
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Close: true} }
		port := collPort(t, dev.ServeTCP(t))
		var stderr collSyncBuffer
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Parallel: 2, Debug: true}, &stderr)
		require.NoError(t, err)
		require.Empty(t, out.Devices)
		require.Contains(t, stderr.String(), "DEBUG [discover] 127.0.0.1: holding read:")
	})

	t.Run("invalid subnet is skipped", func(t *testing.T) {
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{"not-a-cidr"}, Port: 502, Parallel: 2}, nil)
		require.NoError(t, err)
		require.Empty(t, out.Devices)
	})

	t.Run("context already canceled", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		port := collPort(t, dev.ServeTCP(t))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		out, err := CollectDiscover(ctx, config.DiscoverConfig{Subnets: []string{"127.0.0.0/16"}, Port: port, Parallel: 4}, nil)
		require.NoError(t, err)
		require.Empty(t, out.Devices)
		require.Zero(t, dev.RequestCount(0), "no host may be probed after cancellation")
		require.Less(t, time.Since(start), 2*time.Second)
	})
}

// MAC resolution is best effort: a device stays in the result when its MAC cannot be resolved.
func TestCollDiscover_unresolvableMACKeepsDevice(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 1)
	port := collPort(t, dev.ServeTCP(t))
	out, err := CollectDiscover(context.Background(), config.DiscoverConfig{
		Subnets: []string{collLoopback}, Port: port, Parallel: 1, ResolveMAC: true, NetworkInterface: "coll-no-such-if0",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, []types.DiscoverJson{{IP: "127.0.0.1", Port: port, Mac: "", Interface: "coll-no-such-if0"}}, out.Devices)

	_, err = resolveMACAddress("127.0.0.1", "coll-no-such-if0")
	require.Error(t, err)
}

func TestCollDiscover_outputFile(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 1)
	port := collPort(t, dev.ServeTCP(t))

	t.Run("written as a JSON array", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "devices.json")
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Parallel: 1, OutputFile: path}, nil)
		require.NoError(t, err)
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var fromFile []types.DiscoverJson
		require.NoError(t, json.Unmarshal(raw, &fromFile))
		require.Equal(t, out.Devices, fromFile)
		require.Equal(t, []types.DiscoverJson{{IP: "127.0.0.1", Port: port}}, fromFile)
	})

	t.Run("empty result is an empty array", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "devices.json")
		_, err := CollectDiscover(context.Background(), config.DiscoverConfig{
			Subnets: []string{collLoopback}, Port: collPort(t, collClosedAddr(t)), Parallel: 1, OutputFile: path,
		}, nil)
		require.NoError(t, err)
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		require.JSONEq(t, "[]", string(raw))
	})

	t.Run("unwritable path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing-dir", "devices.json")
		out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Parallel: 1, OutputFile: path}, nil)
		require.Nil(t, out)
		collRequireErr(t, err, errs.KindOutput, errs.CodeOutputFileWriteFailed)
	})
}

func TestCollIsValidModbusResponse(t *testing.T) {
	require.True(t, isValidModbusResponse(nil))
	require.True(t, isValidModbusResponse(&gomodbus.ExceptionError{}))
	require.False(t, isValidModbusResponse(errors.New("connection reset")))
	require.False(t, isValidModbusResponse(context.DeadlineExceeded))
}

func TestCollForEachAssignableHost(t *testing.T) {
	collect := func(cidr string) []string {
		var hosts []string
		require.NoError(t, forEachAssignableHost(cidr, func(h string) bool {
			hosts = append(hosts, h)
			return true
		}))
		return hosts
	}
	require.Equal(t, []string{"10.1.2.3"}, collect("10.1.2.3/32"))
	require.Equal(t, []string{"10.0.0.0", "10.0.0.1"}, collect("10.0.0.1/31"), "a /31 has no network or broadcast address")
	require.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, collect("10.0.0.0/30"))
	require.Len(t, collect("192.168.255.0/24"), 254)
	require.Equal(t, "10.0.1.0", collect("10.0.0.0/23")[255], "the host part carries into the next octet")

	t.Run("yield false stops the walk", func(t *testing.T) {
		for _, cidr := range []string{"10.0.0.0/31", "10.0.0.0/24"} {
			calls := 0
			require.NoError(t, forEachAssignableHost(cidr, func(string) bool {
				calls++
				return false
			}))
			require.Equalf(t, 1, calls, "cidr %s", cidr)
		}
	})

	require.Error(t, forEachAssignableHost("10.0.0.300/24", func(string) bool { return true }))

	n, err := EstimateDiscoverHostCount(config.DiscoverConfig{Subnets: []string{"10.0.0.0/30", "10.0.0.0/29", "10.9.9.9/32"}})
	require.NoError(t, err)
	require.Equal(t, 7, n, "overlapping subnets are counted once")
	_, err = EstimateDiscoverHostCount(config.DiscoverConfig{Subnets: []string{"10.0.0.0/30", "bad"}})
	require.Error(t, err)
}

// collServeExceptionOnAllLoopback answers every Modbus TCP request with an illegal-data-address
// exception on every local address, and returns its port.
func collServeExceptionOnAllLoopback(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				req := make([]byte, 12)
				if _, err := io.ReadFull(conn, req); err != nil {
					return
				}
				_, _ = conn.Write([]byte{req[0], req[1], 0, 0, 0, 3, req[6], req[7] | 0x80, 0x02})
			}()
		}
	}()
	return uint16(ln.Addr().(*net.TCPAddr).Port)
}

// Devices are listed in numeric address order (.2 before .10), whatever order the workers finish in.
func TestCollDiscover_sortsDevicesNumerically(t *testing.T) {
	port := collServeExceptionOnAllLoopback(t)
	// Only Linux routes the whole 127.0.0.0/8 block to the loopback interface out of the box.
	for _, host := range []string{"127.0.0.2", "127.0.0.14"} {
		conn, err := net.DialTimeout("tcp4", net.JoinHostPort(host, strconv.Itoa(int(port))), 300*time.Millisecond)
		if err != nil {
			requireOrSkip(t, fmt.Sprintf("%s is not a usable loopback address on this system: %v", host, err))
		}
		require.NoError(t, conn.Close())
	}

	out, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{"127.0.0.8/29", "127.0.0.0/28"}, Port: port, Parallel: 8}, nil)
	require.NoError(t, err)
	var got []string
	for _, d := range out.Devices {
		require.Equal(t, port, d.Port)
		got = append(got, d.IP)
	}
	want := make([]string, 0, 14)
	for i := 1; i <= 14; i++ {
		want = append(want, "127.0.0."+strconv.Itoa(i))
	}
	require.Equal(t, want, got)
}

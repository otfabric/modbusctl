package modbus

import (
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// coreTimeoutMS is the request timeout used by tests that expect an answer.
const coreTimeoutMS = 2000

// coreShortTimeoutMS is the request timeout used by tests that wait for a timeout.
const coreShortTimeoutMS = 200

// coreTCP serves d over Modbus TCP and returns the device configuration reaching it as unit 1.
func coreTCP(t *testing.T, d *mbtest.Device) config.DeviceConfig {
	t.Helper()
	return config.DeviceConfig{URL: "tcp://" + d.ServeTCP(t), Unit: 1}
}

// coreClosedURL returns a tcp:// URL on which nothing listens.
func coreClosedURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return "tcp://" + addr
}

// coreMcapPath returns a capture path inside the test's temporary directory.
func coreMcapPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "capture.mbcap")
}

// coreLoadMcap reads back a capture file.
func coreLoadMcap(t *testing.T, path string) ([]types.CaptureRecord, types.CaptureHeader) {
	t.Helper()
	return capfiletest.ReadRecords(t, path)
}

// coreRequireKind asserts that err, once wrapped the way the command runner wraps collector
// errors, has the given kind and code.
func coreRequireKind(t *testing.T, err error, kind errs.Kind, code string) {
	t.Helper()
	require.Error(t, err)
	e, ok := errs.As(WrapCollectError(err))
	require.True(t, ok, "not an *errs.Error: %v", err)
	require.Equal(t, kind, e.Kind, "error: %v", err)
	require.Equal(t, code, e.Code, "error: %v", err)
}

// coreFirstN returns an OnRequest hook that applies act to the first n requests only.
func coreFirstN(n int32, act mbtest.Action) func(mbtest.Request) mbtest.Action {
	var seen atomic.Int32
	return func(mbtest.Request) mbtest.Action {
		if seen.Add(1) <= n {
			return act
		}
		return mbtest.Action{}
	}
}

// coreDropListener accepts one connection at a time, reads the first request and closes both
// the connection and the listener, so that the client can neither finish the request nor
// re-dial. It returns the tcp:// URL to connect to.
func coreDropListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_ = ln.Close()
		_ = conn.Close()
	}()
	return "tcp://" + ln.Addr().String()
}

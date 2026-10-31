// SPDX-License-Identifier: MIT

package modbus

import (
	"bytes"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// collClosedAddr returns a loopback host:port on which nothing listens.
func collClosedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// collPort extracts the numeric port of a host:port address.
func collPort(t *testing.T, addr string) uint16 {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	n, err := strconv.ParseUint(p, 10, 16)
	require.NoError(t, err)
	return uint16(n)
}

// collRequireErr asserts that err is a typed *errs.Error with the given kind and code.
func collRequireErr(t *testing.T, err error, kind errs.Kind, code string) {
	t.Helper()
	_ = collTypedErr(t, err, kind, code)
}

// collTypedErr is collRequireErr for callers that also inspect the message or cause.
func collTypedErr(t *testing.T, err error, kind errs.Kind, code string) *errs.Error {
	t.Helper()
	require.Error(t, err)
	e, ok := errs.As(err)
	require.Truef(t, ok, "expected *errs.Error, got %T: %v", err, err)
	require.Equal(t, kind, e.Kind, "kind of %v", err)
	require.Equal(t, code, e.Code, "code of %v", err)
	return e
}

// collRequireInfo asserts the kind and code of an embedded per-row error.
func collRequireInfo(t *testing.T, info *types.ErrorInfo, kind errs.Kind, code string) {
	t.Helper()
	require.NotNil(t, info, "expected an embedded error")
	require.Equal(t, string(kind), info.Kind, "kind of %q", info.Message)
	require.Equal(t, code, info.Code, "code of %q", info.Message)
	require.NotEmpty(t, info.Message)
}

// collIdentDevice returns a device exposing basic, regular and extended FC43 objects and FC17.
func collIdentDevice() *mbtest.Device {
	d := mbtest.NewDevice()
	d.Identification = map[uint8]string{
		0x00: "OTFabric",
		0x01: "MB-1",
		0x02: "1.2",
		0x03: "https://example.test",
		0x04: "Simulator",
		0x80: "private",
	}
	d.ServerID = &mbtest.ServerID{ID: []byte{0x42}, Running: true}
	return d
}

// collSyncBuffer is a bytes.Buffer that is safe to write from one goroutine and read from another.
type collSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *collSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *collSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

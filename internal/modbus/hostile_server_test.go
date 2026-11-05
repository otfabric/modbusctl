// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/replay"
	"github.com/stretchr/testify/require"
)

// hostileDial connects to the server as a raw TCP client.
func hostileDial(t *testing.T, srv *collServer) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", srv.port), 2*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// mbap frames pdu for unit with the given transaction, protocol and length fields.
func mbap(txn, proto, length uint16, unit byte, pdu []byte) []byte {
	b := binary.BigEndian.AppendUint16(nil, txn)
	b = binary.BigEndian.AppendUint16(b, proto)
	b = binary.BigEndian.AppendUint16(b, length)
	return append(append(b, unit), pdu...)
}

// requireStillServing reads from a fresh, well-behaved client.
func requireStillServing(t *testing.T, srv *collServer) {
	t.Helper()
	got, err := srv.client(t).ReadHoldingRegisters(context.Background(), 21, 10, 2)
	require.NoError(t, err, "the server stopped serving well-behaved clients")
	require.Equal(t, []uint16{5, 6}, got)
}

// TestHostileClient_malformedFrames sends what a fuzzer or an attacker would:
// wrong protocol ids, lengths of 0, 1 and 0xFFFF, a header cut short, an MBAP
// length longer than the frame, garbage, and reads at the top of the address
// space, then checks that the server still answers a normal client.
func TestHostileClient_malformedFrames(t *testing.T) {
	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite())})
	frames := [][]byte{
		mbap(1, 1, 6, 21, readReq(3, 10, 2)),         // protocol id 1
		mbap(2, 0, 0, 21, nil),                       // length 0
		mbap(3, 0, 1, 21, nil),                       // unit only, no PDU
		mbap(4, 0, 0xFFFF, 21, readReq(3, 10, 2)),    // length far beyond the frame
		mbap(5, 0, 6, 21, readReq(3, 65535, 125)),    // address + quantity past 65535
		mbap(6, 0, 6, 21, readReq(3, 0, 0)),          // quantity 0
		mbap(7, 0, 6, 21, readReq(1, 0, 2001)),       // too many coils
		mbap(8, 0, 2, 21, []byte{0x14}),              // FC20 without a byte count
		mbap(9, 0, 3, 21, []byte{0x14, 0xFF}),        // FC20 byte count beyond the PDU
		mbap(10, 0, 4, 21, []byte{0x08, 0x00, 0x01}), // FC08 restart: refused, read-only
		{0xde, 0xad},
		[]byte(strings.Repeat("\xff", 600)),
	}
	for i, f := range frames {
		conn := hostileDial(t, srv)
		_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
		_, _ = conn.Write(f)
		// Whatever the server does (an exception, closing, nothing), it must not
		// hang the read past the deadline or crash.
		_, _ = io.ReadAll(io.LimitReader(conn, 512))
		_ = conn.Close()
		if i%4 == 3 {
			requireStillServing(t, srv)
		}
	}
	requireStillServing(t, srv)
}

// TestHostileClient_floodWithoutReading writes requests and never reads the
// answers; the server must keep serving others.
func TestHostileClient_floodWithoutReading(t *testing.T) {
	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite()), MaxClients: 4})
	conn := hostileDial(t, srv)
	req := mbap(1, 0, 6, 21, readReq(3, 10, 2))
	flood := make([]byte, 0, len(req)*20000)
	for range 20000 {
		flood = append(flood, req...)
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(flood) // stops when the socket buffers are full
	requireStillServing(t, srv)
}

// TestHostileClient_connectionsBeyondMaxClients: connections above --max-clients
// are closed, and a well-behaved client is served once the others leave.
func TestHostileClient_connectionsBeyondMaxClients(t *testing.T) {
	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite()), MaxClients: 2})
	var idle []net.Conn
	for range 6 {
		idle = append(idle, hostileDial(t, srv))
	}
	for _, c := range idle {
		_ = c.Close()
	}
	require.Eventually(t, func() bool {
		c, err := gomodbus.New(gomodbus.Config{URL: fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), Timeout: 500 * time.Millisecond, Logger: gomodbus.NopLogger()})
		if err != nil || c.Open() != nil {
			return false
		}
		defer func() { _ = c.Close() }()
		_, err = c.ReadHoldingRegisters(context.Background(), 21, 10, 2)
		return err == nil
	}, 10*time.Second, 50*time.Millisecond)
}

// TestCaptureHandler_fileRecordRequestCount: the handler rebuilds the FC20
// request PDU from the sub-requests; a count that would not fit its byte count
// is refused instead of wrapping (go-modbus bounds it before, this is the
// handler's own guard).
func TestCaptureHandler_fileRecordRequestCount(t *testing.T) {
	c := collSite()
	d, err := replay.Build(c, 1, c.End.At, replay.Options{})
	require.NoError(t, err)
	h := &captureHandler{devices: map[uint8]*replay.Device{21: d}, now: func() capture.Ticks { return c.End.At }}
	for _, n := range []int{0, 36, 40} {
		req := &gomodbus.ReadFileRecordsRequest{UnitID: 21, Requests: make([]gomodbus.FileRecordRequest, n)}
		_, err := h.HandleReadFileRecords(context.Background(), req)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataValue, "%d sub-requests", n)
	}
	req := &gomodbus.ReadFileRecordsRequest{UnitID: 21, Requests: []gomodbus.FileRecordRequest{{FileNumber: 1, RecordNumber: 0, RecordLength: 1}}}
	_, err = h.HandleReadFileRecords(context.Background(), req)
	require.ErrorIs(t, err, gomodbus.ErrIllegalFunction, "not recorded")
}

// TestParseSpeed_refusesNonFinite: NaN and Inf passed the "positive" check
// (NaN <= 0 is false) and made the replay time undefined.
func TestParseSpeed_refusesNonFinite(t *testing.T) {
	for _, s := range []string{"NaN", "nan", "Inf", "+Inf", "infinity", "-1", "0", "1e300", fmt.Sprint(MaxSpeed + 1), "x"} {
		_, err := ParseSpeed(s)
		require.Error(t, err, s)
	}
	for _, s := range []string{"", "1", "0.001", "60", fmt.Sprint(MaxSpeed)} {
		_, err := ParseSpeed(s)
		require.NoError(t, err, s)
	}
	require.False(t, math.IsNaN(MaxSpeed))
}

// SPDX-License-Identifier: MIT

// Package capfiletest builds and reads MBCAP captures in tests, in terms of the
// register-block records ([types.CaptureRecord]) the tests have always used.
package capfiletest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/mbcap"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/types"
)

// Path returns a capture path in the test's temporary directory.
func Path(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name)
}

// Capture returns the MBCAP capture of the given header and records: one
// target (the header's endpoint and unit), one poll stream, and one sample per
// record, with the request (function, start, count) and response (function,
// byte count, data) PDUs of the header's function. A record's time is its
// ResponseTimestamp relative to the header's StartTime when both are set, and
// one second per iteration otherwise.
func Capture(h types.CaptureHeader, recs ...types.CaptureRecord) *capture.Capture {
	start := time.Unix(0, h.StartTime).UTC()
	if h.StartTime == 0 {
		start = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	}
	c := &capture.Capture{
		Header:  capture.Header{TickNS: capture.DefaultTickNS, WallStart: start, Producer: "capfiletest"},
		Targets: []capture.Target{{ID: 1, Transport: capture.TransportTCP, Host: h.IP, Port: h.Port, Unit: h.Unit}},
		Streams: []capture.Stream{{ID: 1, Target: 1, Role: capture.RolePoll}},
	}
	var end capture.Ticks
	for _, r := range recs {
		at := capture.Ticks(r.Iteration) * 1_000_000
		if r.ResponseTimestamp > 0 && h.StartTime > 0 && r.ResponseTimestamp >= h.StartTime {
			at = capture.Ticks((r.ResponseTimestamp - h.StartTime) / 1000)
		}
		// The request is sized from the data, as a device would answer it: one word per
		// register (an odd last byte padded), one bit per coil.
		data := r.Data
		qty := r.RegisterCount
		switch h.Function {
		case 3, 4:
			if len(data)%2 == 1 {
				data = append(append([]byte(nil), data...), 0)
			}
			qty = uint16(len(data) / 2)
		case 1, 2:
			if (int(qty)+7)/8 != len(data) {
				qty = uint16(8 * len(data))
			}
		}
		req := []byte{h.Function, byte(r.StartAddress >> 8), byte(r.StartAddress), byte(qty >> 8), byte(qty)}
		resp := append([]byte{h.Function, byte(len(data))}, data...)
		c.Samples = append(c.Samples, capture.Sample{Stream: 1, At: at, Request: req, Response: resp})
		end = max(end, at)
	}
	c.End = capture.End{Known: true, At: end + 1_000_000}
	return c
}

// Write writes c as an MBCAP file at path.
func Write(t testing.TB, path string, c *capture.Capture) string {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	w, err := mbcap.NewWriter(f, c.Header, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteTo(w); err != nil {
		t.Fatal(err)
	}
	return path
}

// WriteRecords writes the capture of h and recs (see [Capture]) to path.
func WriteRecords(t testing.TB, path string, h types.CaptureHeader, recs ...types.CaptureRecord) string {
	t.Helper()
	return Write(t, path, Capture(h, recs...))
}

// ReadRecords reads the capture at path as register-block records of its
// only target and function (see [capfile.Records]).
func ReadRecords(t testing.TB, path string) ([]types.CaptureRecord, types.CaptureHeader) {
	t.Helper()
	l, err := capfile.Load(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	recs, h, err := capfile.Records(l, capfile.Selection{})
	if err != nil {
		t.Fatalf("records of %s: %v", path, err)
	}
	return recs, h
}

// Load reads the whole capture at path.
func Load(t testing.TB, path string) *capfile.Loaded {
	t.Helper()
	l, err := capfile.Load(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return l
}

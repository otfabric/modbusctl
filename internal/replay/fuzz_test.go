// SPDX-License-Identifier: MIT

package replay

import (
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
)

// FuzzRead feeds arbitrary request PDUs, as a hostile client sends them, to a
// device built from a capture. An answer to a read must be a well-formed
// response that fits one PDU.
func FuzzRead(f *testing.F) {
	for _, s := range [][]byte{
		req(3, 10, 2), req(3, 10, 0), req(3, 65535, 2), req(3, 0, 126), req(1, 0, 10), req(1, 0, 2001),
		{0x07}, {0x2B, 0x0E, 0x01, 0x00}, {}, {0x03}, {0x03, 0x00}, {0x11}, {0x14, 0x07, 0x06, 0, 1, 0, 0, 0, 1},
	} {
		f.Add(s, uint64(200))
	}
	d, err := Build(testCapture(), 1, 1000, Options{})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, pdu []byte, at uint64) {
		resp, ok := d.Read(capture.Ticks(at), pdu)
		if !ok {
			return
		}
		if len(resp) == 0 || len(resp) > 253 || resp[0] != pdu[0] {
			t.Fatalf("request % x: response % x", pdu, resp)
		}
		if fc := pdu[0]; fc >= 1 && fc <= 4 && int(resp[1]) != len(resp)-2 {
			t.Fatalf("request % x: byte count %d for %d data bytes", pdu, resp[1], len(resp)-2)
		}
	})
}

// FuzzClock checks the replay time for any speed, window and elapsed time.
func FuzzClock(f *testing.F) {
	f.Add(uint64(1000), 1.0, uint64(0), uint64(1000), 0, int64(time.Second))
	f.Add(uint64(1), 1e300, uint64(5), uint64(6), 3, int64(1<<62))
	f.Fuzz(func(t *testing.T, tick uint64, speed float64, from, to uint64, loops int, elapsed int64) {
		c := Clock{TickNS: tick, Speed: speed, From: capture.Ticks(from), To: capture.Ticks(to), Loops: loops}
		got := c.At(time.Duration(elapsed))
		if to > from && (got < c.From || got > c.To) {
			t.Fatalf("%+v at %v: T = %d outside the window", c, time.Duration(elapsed), got)
		}
	})
}

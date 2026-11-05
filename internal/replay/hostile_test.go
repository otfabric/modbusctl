// SPDX-License-Identifier: MIT

package replay

import (
	"math"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
)

// TestRead_quantityBeyondOneResponse: Read sized the byte count with byte(2*qty), so
// a request for 200 recorded registers came back with byte count 144 (400 mod 256)
// and 400 data bytes. Quantities above the per-function limits are not answered.
func TestRead_quantityBeyondOneResponse(t *testing.T) {
	vals := make([]uint16, 100)
	c := &capture.Capture{
		Header:  capture.Header{TickNS: 1000},
		Targets: []capture.Target{{ID: 1, Unit: 1}},
		Streams: []capture.Stream{{ID: 1, Target: 1}},
		Samples: []capture.Sample{
			{Stream: 1, At: 1, Request: req(3, 0, 100), Response: regs(3, vals...)},
			{Stream: 1, At: 1, Request: req(3, 100, 100), Response: regs(3, vals...)},
			{Stream: 1, At: 1, Request: req(1, 0, 2000), Response: append([]byte{1, 250}, make([]byte, 250)...)},
			{Stream: 1, At: 1, Request: req(1, 2000, 2000), Response: append([]byte{1, 250}, make([]byte, 250)...)},
		},
	}
	d, err := Build(c, 1, 10, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		fc        byte
		qty       uint16
		ok        bool
		byteCount int
	}{
		{3, 125, true, 250}, {3, 126, false, 0}, {3, 200, false, 0},
		{1, 2000, true, 250}, {1, 2001, false, 0}, {1, 4000, false, 0},
	} {
		resp, ok := d.Read(5, req(q.fc, 0, q.qty))
		if ok != q.ok {
			t.Fatalf("FC%02d x%d: ok = %v, want %v", q.fc, q.qty, ok, q.ok)
		}
		if ok && (int(resp[1]) != q.byteCount || len(resp) != 2+q.byteCount) {
			t.Fatalf("FC%02d x%d: byte count %d, %d bytes", q.fc, q.qty, resp[1], len(resp))
		}
	}
}

// TestClock_extremeSpeedsStayInTheWindow: the replay time is always in [From, To].
func TestClock_extremeSpeedsStayInTheWindow(t *testing.T) {
	for _, speed := range []float64{math.Inf(1), math.NaN(), math.MaxFloat64, 1e300, 1e18, 365 * 24 * 3600} {
		for _, loops := range []int{0, 1, 3} {
			c := Clock{TickNS: 1, Speed: speed, From: 100, To: 1000, Loops: loops}
			for _, el := range []time.Duration{1, time.Second, time.Duration(math.MaxInt64)} {
				if got := c.At(el); got < c.From || got > c.To {
					t.Fatalf("speed %g loops %d elapsed %v: T = %d outside [%d, %d]", speed, loops, el, got, c.From, c.To)
				}
			}
		}
	}
	// A normal replay still loops through the window.
	c := Clock{TickNS: 1000, Speed: 1, From: 0, To: 1000}
	if got := c.At(1500 * time.Microsecond); got != 500 {
		t.Fatalf("T = %d, want 500", got)
	}
}

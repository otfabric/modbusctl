package replay

import (
	"bytes"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
)

func regs(fc byte, vals ...uint16) []byte {
	b := []byte{fc, byte(2 * len(vals))}
	for _, v := range vals {
		b = append(b, byte(v>>8), byte(v))
	}
	return b
}

func req(fc byte, start, qty uint16) []byte {
	return []byte{fc, byte(start >> 8), byte(start), byte(qty >> 8), byte(qty)}
}

func testCapture() *capture.Capture {
	return &capture.Capture{
		Header:  capture.Header{TickNS: 1000, WallStart: time.Unix(0, 0).UTC(), Exceptions: true},
		Targets: []capture.Target{{ID: 1, Unit: 21}, {ID: 2, Unit: 3}},
		Streams: []capture.Stream{{ID: 1, Target: 1}, {ID: 2, Target: 1}, {ID: 3, Target: 2}},
		Samples: []capture.Sample{
			{Stream: 1, At: 100, Request: req(3, 10, 2), Response: regs(3, 1, 2)},
			// Another stream reads an overlapping block, a little later.
			{Stream: 2, At: 150, Request: req(3, 11, 2), Response: regs(3, 20, 30)},
			{Stream: 1, At: 200, Request: req(3, 10, 2), Response: regs(3, 5, 2)},
			// An exception is not hosted data.
			{Stream: 1, At: 250, Request: req(3, 10, 2), Response: []byte{0x83, 2}},
			{Stream: 1, At: 120, Request: req(1, 0, 10), Response: []byte{0x01, 2, 0b1100_1101, 0b01}},
			{Stream: 1, At: 130, Request: []byte{0x07}, Response: []byte{0x07, 0x6d}},
			{Stream: 1, At: 230, Request: []byte{0x07}, Response: []byte{0x07, 0x00}},
			// FC43 read in two pages, then a later read with a new revision.
			{Stream: 1, At: 300, Request: []byte{0x2B, 0x0E, 0x01, 0x00},
				Response: []byte{0x2B, 0x0E, 0x01, 0x01, 0xFF, 0x02, 0x02, 0x00, 0x04, 'A', 'C', 'M', 'E', 0x01, 0x02, 'X', '1'}},
			{Stream: 1, At: 310, Request: []byte{0x2B, 0x0E, 0x01, 0x02},
				Response: []byte{0x2B, 0x0E, 0x01, 0x01, 0x00, 0x00, 0x01, 0x02, 0x03, '1', '.', '0'}},
			{Stream: 1, At: 400, Request: []byte{0x2B, 0x0E, 0x04, 0x02},
				Response: []byte{0x2B, 0x0E, 0x04, 0x81, 0x00, 0x00, 0x01, 0x02, 0x03, '1', '.', '1'}},
			// Target 2 is separate.
			{Stream: 3, At: 50, Request: req(4, 0, 1), Response: regs(4, 7)},
		},
	}
}

func build(t *testing.T, target uint32, opts Options) *Device {
	t.Helper()
	d, err := Build(testCapture(), target, 1000, opts)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSampleAndHold(t *testing.T) {
	d := build(t, 1, Options{})
	for _, tc := range []struct {
		at   capture.Ticks
		want []uint16
	}{
		{0, []uint16{1, 2, 30}},   // before any sample: HoldFirst
		{100, []uint16{1, 2, 30}}, // exactly on the first sample; 12 holds its first sample (150)
		{149, []uint16{1, 2, 30}},
		{150, []uint16{1, 20, 30}}, // the second stream's block at 150
		{199, []uint16{1, 20, 30}},
		{200, []uint16{5, 2, 30}}, // stream 1 again; the exception at 250 changes nothing
		{999, []uint16{5, 2, 30}},
		{5000, []uint16{5, 2, 30}}, // after the end the last values hold
	} {
		got, ok := d.Values(tc.at, 3, 10, 3)
		if !ok || !equal(got, tc.want) {
			t.Errorf("at %d: %v %v, want %v", tc.at, got, ok, tc.want)
		}
	}
	// Address 9 and 13 were never recorded: a range touching them has no answer.
	if _, ok := d.Values(200, 3, 9, 2); ok {
		t.Error("a range with an unrecorded address was answered")
	}
	if _, ok := d.Values(200, 3, 12, 2); ok {
		t.Error("a range with an unrecorded address was answered")
	}
	// Input registers were recorded for target 2 only.
	if _, ok := d.Values(200, 4, 0, 1); ok {
		t.Error("target 1 answered target 2's data")
	}
	if got, ok := build(t, 2, Options{}).Values(0, 4, 0, 1); !ok || got[0] != 7 {
		t.Errorf("target 2: %v %v", got, ok)
	}
}

func TestSpan(t *testing.T) {
	first, end := build(t, 1, Options{}).Span()
	if first != 100 || end != 1000 {
		t.Errorf("span %d..%d, want 100..1000", first, end)
	}
	if n := build(t, 2, Options{}).Samples(); n != 1 {
		t.Errorf("target 2 holds %d samples", n)
	}
	if _, err := Build(testCapture(), 9, 1000, Options{}); err == nil {
		t.Error("a missing target was accepted")
	}
}

func TestAbsentUntilSampled(t *testing.T) {
	d := build(t, 1, Options{BeforeFirst: AbsentUntilSampled})
	if _, ok := d.Values(99, 3, 10, 1); ok {
		t.Error("a value before its first sample was answered")
	}
	if got, ok := d.Values(100, 3, 10, 1); !ok || got[0] != 1 {
		t.Errorf("exactly on the first sample: %v %v", got, ok)
	}
	if _, ok := d.Values(149, 3, 12, 1); ok {
		t.Error("address 12 before its first sample was answered")
	}
}

func TestStreamFilter(t *testing.T) {
	d := build(t, 1, Options{Streams: []uint32{1}})
	if _, ok := d.Values(500, 3, 12, 1); ok {
		t.Error("a sample of an excluded stream was used")
	}
}

func TestReadBuildsResponses(t *testing.T) {
	d := build(t, 1, Options{})
	if got, ok := d.Read(200, req(3, 10, 3)); !ok || !bytes.Equal(got, regs(3, 5, 2, 30)) {
		t.Errorf("FC03: % x %v", got, ok)
	}
	// Coils LSB first, a range inside the recorded one.
	if got, ok := d.Read(200, req(1, 2, 4)); !ok || !bytes.Equal(got, []byte{0x01, 1, 0b0011}) {
		t.Errorf("FC01: % x %v", got, ok)
	}
	if got, ok := d.Read(200, []byte{0x07}); !ok || !bytes.Equal(got, []byte{0x07, 0x6d}) {
		t.Errorf("FC07 at 200: % x %v", got, ok)
	}
	if got, ok := d.Read(230, []byte{0x07}); !ok || !bytes.Equal(got, []byte{0x07, 0x00}) {
		t.Errorf("FC07 at 230: % x %v", got, ok)
	}
	if _, ok := d.Read(200, []byte{0x11}); ok {
		t.Error("an unrecorded function was answered")
	}
	if !d.HasFunction(0x07) || d.HasFunction(0x11) || !d.HasFunction(0x2B) || d.HasFunction(4) {
		t.Error("HasFunction")
	}
}

func TestIdentification(t *testing.T) {
	d := build(t, 1, Options{})
	conf, objs, ok := d.Identification(350)
	want := []Object{{0, []byte("ACME")}, {1, []byte("X1")}, {2, []byte("1.0")}}
	if !ok || conf != 0x01 || !sameObjects(objs, want) {
		t.Fatalf("at 350: %x %v %v", conf, objs, ok)
	}
	conf, objs, _ = d.Identification(400)
	want[2].Value = []byte("1.1")
	if conf != 0x81 || !sameObjects(objs, want) {
		t.Fatalf("at 400: %x %v", conf, objs)
	}
	if _, _, ok := build(t, 2, Options{}).Identification(400); ok {
		t.Fatal("target 2 has no identification")
	}
}

func TestClock(t *testing.T) {
	c := Clock{TickNS: 1000, Speed: 1, From: 100, To: 1100, Loops: 0}
	for _, tc := range []struct {
		elapsed time.Duration
		want    capture.Ticks
	}{
		{0, 100},
		{500 * time.Microsecond, 600},
		{999 * time.Microsecond, 1099},
		{1000 * time.Microsecond, 100}, // the seam: back to the start
		{2500 * time.Microsecond, 600},
	} {
		if got := c.At(tc.elapsed); got != tc.want {
			t.Errorf("loop forever, %v: %d, want %d", tc.elapsed, got, tc.want)
		}
	}
	c.Loops = 2
	if got := c.At(1500 * time.Microsecond); got != 600 {
		t.Errorf("second loop: %d", got)
	}
	if got := c.At(2000 * time.Microsecond); got != 1100 {
		t.Errorf("after two loops: %d, want the end", got)
	}
	c.Loops, c.Speed = 0, 10
	if got := c.At(50 * time.Microsecond); got != 600 {
		t.Errorf("speed 10: %d", got)
	}
	c.Speed = 0.5
	if got := c.At(1000 * time.Microsecond); got != 600 {
		t.Errorf("speed 0.5: %d", got)
	}
	c.Speed = 0
	if got := c.At(time.Hour); got != 100 {
		t.Errorf("speed 0 is static at From: %d", got)
	}
}

func equal(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameObjects(a, b []Object) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || !bytes.Equal(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

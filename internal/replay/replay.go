// Package replay answers Modbus requests from a capture as of a replay time T:
// the data core of a virtual device. It applies sample and hold (a value holds
// from its sample until the next sample of the same address or request), never
// invents data, and leaves the answer to unrecorded requests to the caller. It
// takes T as an argument and never reads the wall clock; [Clock] maps wall
// time to T for the replay speed and loops.
//
// Values are kept per address for FC01–04, per object for FC43/14 device
// identification, and per exact request PDU for every other function.
// Exception responses in the capture are not hosted data and are ignored.
package replay

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/otfabric/go-modbus-capture/capture"
)

// ErrUnknownTarget is returned by [Build] for a target the capture does not define.
var ErrUnknownTarget = errors.New("unknown target")

// BeforeFirst says what a value is before its first sample.
type BeforeFirst int

const (
	// HoldFirst serves the first sample from the start of the capture: data
	// read once, such as identity, exists from second 0.
	HoldFirst BeforeFirst = iota
	// AbsentUntilSampled treats a value as unrecorded until its first sample.
	AbsentUntilSampled
)

// Options configure a Device.
type Options struct {
	BeforeFirst BeforeFirst
	// Streams restricts the device to these streams (nil: all streams of the
	// target).
	Streams []uint32
}

type point struct {
	at capture.Ticks
	v  uint16
}

type respPoint struct {
	at   capture.Ticks
	resp []byte
}

type objPoint struct {
	at    capture.Ticks
	value []byte
}

// Object is one device identification object.
type Object struct {
	ID    uint8
	Value []byte
}

// Device is one target of a capture, answering from its recorded samples.
// It is immutable after Build and safe for concurrent use.
type Device struct {
	target capture.Target
	opts   Options
	addrs  [5]map[uint16][]point // FC01–04, by address
	reqs   map[string][]respPoint
	objs   map[uint8][]objPoint
	conf   []point // FC43 conformity level
	first  capture.Ticks
	end    capture.Ticks
	n      int
}

// Build collects the samples of target from c. end is the time values hold
// until (the capture end, or the last sample time when the end is unknown).
func Build(c *capture.Capture, target uint32, end capture.Ticks, opts Options) (*Device, error) {
	d := &Device{opts: opts, reqs: map[string][]respPoint{}, objs: map[uint8][]objPoint{}, end: end}
	found := false
	for _, t := range c.Targets {
		if t.ID == target {
			d.target, found = t, true
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: the capture has no target %d", ErrUnknownTarget, target)
	}
	for fc := 1; fc <= 4; fc++ {
		d.addrs[fc] = map[uint16][]point{}
	}
	streams := map[uint32]bool{}
	allowed := map[uint32]bool{}
	for _, s := range opts.Streams {
		allowed[s] = true
	}
	for _, s := range c.Streams {
		if s.Target == target && (len(allowed) == 0 || allowed[s.ID]) {
			streams[s.ID] = true
		}
	}
	// Samples in time order (stable: samples at the same time keep file order).
	idx := make([]int, 0, len(c.Samples))
	for i := range c.Samples {
		s := &c.Samples[i]
		if streams[s.Stream] && !s.IsException() {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool { return c.Samples[idx[a]].At < c.Samples[idx[b]].At })
	d.first = end
	for _, i := range idx {
		s := &c.Samples[i]
		d.add(s)
		d.first = min(d.first, s.At)
		d.n++
	}
	if d.n == 0 {
		d.first = 0
	}
	return d, nil
}

func (d *Device) add(s *capture.Sample) {
	fc := s.Function()
	switch {
	case fc >= 1 && fc <= 4:
		if av, ok := capture.SplitAddresses(s); ok {
			m := d.addrs[fc]
			for i, v := range av.Values {
				a := av.Start + uint16(i)
				m[a] = appendPoint(m[a], point{s.At, v})
			}
		}
	case fc == 0x2B && len(s.Request) >= 2 && s.Request[1] == 0x0E:
		d.addIdentity(s)
	default:
		key := string(s.Request)
		pts := d.reqs[key]
		if n := len(pts); n > 0 && bytes.Equal(pts[n-1].resp, s.Response) {
			return // unchanged: the earlier sample already holds
		}
		d.reqs[key] = append(pts, respPoint{s.At, s.Response})
	}
}

func appendPoint(pts []point, p point) []point {
	if n := len(pts); n > 0 && pts[n-1].v == p.v {
		return pts
	}
	return append(pts, p)
}

// addIdentity merges one FC43/14 page: 2B 0E code conformity more next count
// (id len value)*.
func (d *Device) addIdentity(s *capture.Sample) {
	r := s.Response
	if len(r) < 7 || r[0] != 0x2B || r[1] != 0x0E {
		return
	}
	d.conf = appendPoint(d.conf, point{s.At, uint16(r[3])})
	count, p := int(r[6]), 7
	for i := 0; i < count && p+2 <= len(r); i++ {
		id, n := r[p], int(r[p+1])
		if p+2+n > len(r) {
			return
		}
		v := append([]byte(nil), r[p+2:p+2+n]...)
		pts := d.objs[id]
		if k := len(pts); k == 0 || !bytes.Equal(pts[k-1].value, v) {
			d.objs[id] = append(pts, objPoint{s.At, v})
		}
		p += 2 + n
	}
}

// Target returns the target the device hosts.
func (d *Device) Target() capture.Target { return d.target }

// Samples returns the number of samples the device holds.
func (d *Device) Samples() int { return d.n }

// Span returns the first sample time and the time values hold until.
func (d *Device) Span() (first, end capture.Ticks) { return d.first, d.end }

// index returns the position of the latest entry at or before t among n
// entries whose times at(i) increase, applying BeforeFirst; -1 when there is
// none.
func (d *Device) index(n int, at func(int) capture.Ticks, t capture.Ticks) int {
	if n == 0 {
		return -1
	}
	i := sort.Search(n, func(i int) bool { return at(i) > t }) - 1
	if i < 0 && d.opts.BeforeFirst == HoldFirst {
		return 0
	}
	return i
}

func (d *Device) value(pts []point, t capture.Ticks) (uint16, bool) {
	i := d.index(len(pts), func(i int) capture.Ticks { return pts[i].at }, t)
	if i < 0 {
		return 0, false
	}
	return pts[i].v, true
}

// Values returns the values of qty addresses from start for function fc
// (1–4) at time t: 0/1 for coils and discrete inputs, words for registers.
// ok is false when any of them was never recorded (by t).
func (d *Device) Values(t capture.Ticks, fc uint8, start, qty uint16) (vals []uint16, ok bool) {
	if fc < 1 || fc > 4 || qty == 0 || uint32(start)+uint32(qty) > 1<<16 {
		return nil, false
	}
	m := d.addrs[fc]
	vals = make([]uint16, qty)
	for i := range vals {
		v, ok := d.value(m[start+uint16(i)], t)
		if !ok {
			return nil, false
		}
		vals[i] = v
	}
	return vals, true
}

// Lookup is Values for each address on its own: have[i] reports whether
// address start+i was recorded (by t); vals[i] is 0 where it was not.
func (d *Device) Lookup(t capture.Ticks, fc uint8, start, qty uint16) (vals []uint16, have []bool) {
	if fc < 1 || fc > 4 || qty == 0 || uint32(start)+uint32(qty) > 1<<16 {
		return nil, nil
	}
	m := d.addrs[fc]
	vals, have = make([]uint16, qty), make([]bool, qty)
	for i := range vals {
		vals[i], have[i] = d.value(m[start+uint16(i)], t)
	}
	return vals, have
}

// HasFunction reports whether the device holds any sample of fc.
func (d *Device) HasFunction(fc uint8) bool {
	switch {
	case fc >= 1 && fc <= 4:
		return len(d.addrs[fc]) > 0
	case fc == 0x2B && len(d.objs) > 0:
		return true
	}
	for k := range d.reqs {
		if k[0] == fc {
			return true
		}
	}
	return false
}

// Response returns the response recorded for exactly req at time t, for
// functions kept per request (all but FC01–04 and FC43/14).
func (d *Device) Response(t capture.Ticks, req []byte) ([]byte, bool) {
	pts := d.reqs[string(req)]
	i := d.index(len(pts), func(i int) capture.Ticks { return pts[i].at }, t)
	if i < 0 {
		return nil, false
	}
	return pts[i].resp, true
}

// Identification returns the device identification objects known at time t,
// in object id order, and the latest conformity level. ok is false when none
// were recorded (by t).
func (d *Device) Identification(t capture.Ticks) (conformity uint8, objects []Object, ok bool) {
	ids := make([]int, 0, len(d.objs))
	for id := range d.objs {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		pts := d.objs[uint8(id)]
		if i := d.index(len(pts), func(i int) capture.Ticks { return pts[i].at }, t); i >= 0 {
			objects = append(objects, Object{ID: uint8(id), Value: pts[i].value})
		}
	}
	if len(objects) == 0 {
		return 0, nil, false
	}
	c, _ := d.value(d.conf, t)
	return uint8(c), objects, true
}

// Read answers a request PDU at time t with a response PDU built from the
// recorded data: FC01–04 per address, other functions per request. FC43
// device identification is paged by the server and is not answered here
// (see Identification). ok is false when the capture holds nothing for req.
func (d *Device) Read(t capture.Ticks, req []byte) (resp []byte, ok bool) {
	if len(req) == 0 {
		return nil, false
	}
	fc := req[0]
	if fc < 1 || fc > 4 {
		if fc == 0x2B {
			return nil, false
		}
		return d.Response(t, req)
	}
	if len(req) != 5 {
		return nil, false
	}
	start := uint16(req[1])<<8 | uint16(req[2])
	qty := uint16(req[3])<<8 | uint16(req[4])
	vals, ok := d.Values(t, fc, start, qty)
	if !ok {
		return nil, false
	}
	if fc <= 2 {
		data := make([]byte, (int(qty)+7)/8)
		for i, v := range vals {
			if v != 0 {
				data[i/8] |= 1 << (i % 8)
			}
		}
		return append([]byte{fc, byte(len(data))}, data...), true
	}
	resp = []byte{fc, byte(2 * qty)}
	for _, v := range vals {
		resp = append(resp, byte(v>>8), byte(v))
	}
	return resp, true
}

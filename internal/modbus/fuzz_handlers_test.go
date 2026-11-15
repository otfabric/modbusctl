// SPDX-License-Identifier: MIT

package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/go-modbus-capture/mbcap"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/replay"
)

// replayFunctionsCapture holds a sample of every function the replay server answers: FC01–04
// per address, FC07, FC08, FC0B, FC0C, FC11, FC14 and FC18 per request, FC43 per object.
func replayFunctionsCapture() *capture.Capture {
	return &capture.Capture{
		Header:  capture.Header{TickNS: 1000, WallStart: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		Targets: []capture.Target{{ID: 1, Unit: 5}, {ID: 2, Unit: 6}},
		Streams: []capture.Stream{{ID: 1, Target: 1}, {ID: 2, Target: 2}},
		Samples: []capture.Sample{
			{Stream: 1, At: 0, Request: readReq(1, 0, 10), Response: []byte{0x01, 2, 0xA5, 0x01}},
			{Stream: 1, At: 0, Request: readReq(2, 4, 3), Response: []byte{0x02, 1, 0x05}},
			{Stream: 1, At: 0, Request: readReq(3, 10, 2), Response: regsResp(3, 1, 2)},
			{Stream: 1, At: 5, Request: readReq(4, 0, 1), Response: regsResp(4, 9)},
			{Stream: 1, At: 5, Request: []byte{0x07}, Response: []byte{0x07, 0x6d}},
			{Stream: 1, At: 5, Request: []byte{0x08, 0x00, 0x0B, 0x00, 0x00}, Response: []byte{0x08, 0x00, 0x0B, 0x00, 0x07}},
			{Stream: 1, At: 5, Request: []byte{0x08, 0x00, 0x00, 0x12, 0x34}, Response: []byte{0x08, 0x00, 0x00, 0x12, 0x34}},
			{Stream: 1, At: 5, Request: []byte{0x0B}, Response: []byte{0x0B, 0x00, 0x00, 0x01, 0x08}},
			{Stream: 1, At: 5, Request: []byte{0x0C}, Response: []byte{0x0C, 0x08, 0x00, 0x00, 0x01, 0x08, 0x01, 0x21, 0x20, 0x00}},
			{Stream: 1, At: 5, Request: []byte{0x11}, Response: []byte{0x11, 0x03, 0x2A, 'x', 0xFF}},
			{Stream: 1, At: 5, Request: []byte{0x14, 0x07, 0x06, 0x00, 0x01, 0x00, 0x02, 0x00, 0x02},
				Response: []byte{0x14, 0x06, 0x05, 0x06, 0x12, 0x34, 0x56, 0x78}},
			{Stream: 1, At: 5, Request: []byte{0x18, 0x04, 0xD2}, Response: []byte{0x18, 0x00, 0x06, 0x00, 0x02, 0x00, 0x0A, 0x00, 0x0B}},
			{Stream: 2, At: 7, Request: []byte{0x2B, 0x0E, 0x01, 0x00},
				Response: []byte{0x2B, 0x0E, 0x01, 0x01, 0x00, 0x00, 0x02, 0x00, 0x04, 'A', 'C', 'M', 'E', 0x01, 0x02, 'X', '1'}},
			{Stream: 1, At: 9, Request: readReq(3, 10, 2), Response: regsResp(3, 3, 4)},
		},
		End: capture.End{Known: true, At: 20},
	}
}

// FuzzReplayHandlers loads arbitrary capture files, as `server static` and `server replay`
// do, and asks the replay server's handlers for every request the capture holds, at the time
// of each sample and at the end, with each --missing mode. Invariants: an answer of FC07,
// FC08, FC0B, FC0C, FC11, FC14 or FC18 is the response recorded for that request, so its byte
// counts match its length; FC01–04 answer with as many values as were asked for; FC43
// objects fit their one-byte length; and an error is one that the server answers with an
// exception or with silence.
func FuzzReplayHandlers(f *testing.F) {
	for _, c := range []*capture.Capture{replayFunctionsCapture(), collSite()} {
		var b bytes.Buffer
		w, err := mbcap.NewWriter(&b, c.Header, nil)
		if err != nil {
			f.Fatal(err)
		}
		if err := c.WriteTo(w); err != nil {
			f.Fatal(err)
		}
		f.Add(b.Bytes())
		b.Reset()
		jw, err := jsonl.NewWriter(&b, c.Header)
		if err != nil {
			f.Fatal(err)
		}
		if err := c.WriteTo(jw); err != nil {
			f.Fatal(err)
		}
		f.Add(b.Bytes())
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "in")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		l, err := capfile.Load(path)
		if err != nil {
			return
		}
		end := l.End()
		unitOf := map[uint32]uint8{}
		devices := map[uint8]*replay.Device{}
		for _, tg := range l.Targets {
			unitOf[tg.ID] = tg.Unit
			if devices[tg.Unit] != nil {
				continue // the server refuses two targets on one unit
			}
			if d, err := replay.Build(l.Capture, tg.ID, end, replay.Options{}); err == nil {
				devices[tg.Unit] = d
			}
		}
		target := l.TargetOf()
		samples := l.Samples[:min(len(l.Samples), 64)]
		times := []capture.Ticks{0, end}
		for _, s := range samples[:min(len(samples), 8)] {
			times = append(times, s.At)
		}
		for _, missing := range []Missing{MissingException, MissingZero, MissingSilence} {
			for _, at := range times {
				h := &captureHandler{devices: devices, now: func() capture.Ticks { return at }, missing: missing}
				for _, s := range samples {
					unit := unitOf[target[s.Stream]]
					checkReplayAnswer(t, h, devices[unit], at, unit, s.Request)
				}
			}
		}
	})
}

// checkReplayAnswer asks h for req on unit at time at and checks the answer against what the
// device d recorded (d is nil for a unit the server does not answer).
func checkReplayAnswer(t *testing.T, h *captureHandler, d *replay.Device, at capture.Ticks, unit uint8, req []byte) {
	t.Helper()
	ctx := context.Background()
	recorded := func() []byte {
		if d == nil {
			t.Fatalf("unit %d: an answer from a unit that is not served", unit)
		}
		r, ok := d.Response(at, req)
		if !ok {
			t.Fatalf("unit %d % X: an answer where nothing is recorded", unit, req)
		}
		return r
	}
	same := func(got []byte) {
		if want := recorded(); !bytes.Equal(got, want) {
			t.Fatalf("unit %d % X: served % X, recorded % X", unit, req, got, want)
		}
	}
	be := binary.BigEndian.AppendUint16
	var err error
	switch fc := req[0]; fc {
	case 1, 2, 3, 4:
		if len(req) < 5 {
			return
		}
		addr, qty := binary.BigEndian.Uint16(req[1:]), binary.BigEndian.Uint16(req[3:])
		// go-modbus answers a quantity outside these limits itself.
		if qty == 0 || fc <= 2 && qty > 2000 || fc >= 3 && qty > 125 {
			return
		}
		var n int
		switch fc {
		case 1:
			var v []bool
			v, err = h.HandleCoils(ctx, &gomodbus.CoilsRequest{UnitID: unit, Addr: addr, Quantity: qty})
			n = len(v)
			if _, werr := h.HandleCoils(ctx, &gomodbus.CoilsRequest{UnitID: unit, Addr: addr, Quantity: qty, IsWrite: true}); !errors.Is(werr, gomodbus.ErrIllegalFunction) {
				t.Fatalf("unit %d: a coil write answered with %v", unit, werr)
			}
		case 2:
			var v []bool
			v, err = h.HandleDiscreteInputs(ctx, &gomodbus.DiscreteInputsRequest{UnitID: unit, Addr: addr, Quantity: qty})
			n = len(v)
		case 3:
			var v []uint16
			v, err = h.HandleHoldingRegisters(ctx, &gomodbus.HoldingRegistersRequest{UnitID: unit, Addr: addr, Quantity: qty})
			n = len(v)
			if _, werr := h.HandleHoldingRegisters(ctx, &gomodbus.HoldingRegistersRequest{UnitID: unit, Addr: addr, Quantity: qty, IsWrite: true}); !errors.Is(werr, gomodbus.ErrIllegalFunction) {
				t.Fatalf("unit %d: a register write answered with %v", unit, werr)
			}
		case 4:
			var v []uint16
			v, err = h.HandleInputRegisters(ctx, &gomodbus.InputRegistersRequest{UnitID: unit, Addr: addr, Quantity: qty})
			n = len(v)
		}
		if err == nil && n != int(qty) {
			t.Fatalf("unit %d FC%02d %d x%d: %d values", unit, fc, addr, qty, n)
		}
	case 0x07:
		var status uint8
		if status, err = h.HandleExceptionStatus(ctx, &gomodbus.ExceptionStatusRequest{UnitID: unit}); err == nil {
			same(append([]byte{0x07, status}, recorded()[2:]...)) // a fixed-length answer: the recorded field
		}
	case 0x08:
		if len(req) < 3 {
			return
		}
		sub := binary.BigEndian.Uint16(req[1:])
		var data []byte
		if data, err = h.HandleDiagnostics(ctx, &gomodbus.DiagnosticsRequest{UnitID: unit, SubFunction: gomodbus.DiagnosticSubFunction(sub), Data: req[3:]}); err == nil {
			same(append(be([]byte{0x08}, sub), data...))
		}
	case 0x0B:
		var r *gomodbus.CommEventCounterResponse
		if r, err = h.HandleCommEventCounter(ctx, &gomodbus.CommEventCounterRequest{UnitID: unit}); err == nil {
			same(append(be(be([]byte{0x0B}, r.Status), r.EventCount), recorded()[5:]...))
		}
	case 0x0C:
		var r *gomodbus.CommEventLogResponse
		if r, err = h.HandleCommEventLog(ctx, &gomodbus.CommEventLogRequest{UnitID: unit}); err == nil {
			same(append(be(be(be([]byte{0x0C, byte(6 + len(r.Events))}, r.Status), r.EventCount), r.MessageCount), r.Events...))
		}
	case 0x11:
		var data []byte
		if data, err = h.HandleReportServerID(ctx, &gomodbus.ReportServerIDRequest{UnitID: unit}); err == nil {
			if len(data) > 251 {
				t.Fatalf("unit %d: FC17 data of %d bytes", unit, len(data))
			}
			same(append([]byte{0x11, byte(len(data))}, data...))
		}
	case 0x14:
		if len(req) < 2 || (len(req)-2)%7 != 0 {
			return
		}
		var subs []gomodbus.FileRecordRequest
		for p := 2; p+7 <= len(req); p += 7 {
			subs = append(subs, gomodbus.FileRecordRequest{FileNumber: binary.BigEndian.Uint16(req[p+1:]),
				RecordNumber: binary.BigEndian.Uint16(req[p+3:]), RecordLength: binary.BigEndian.Uint16(req[p+5:])})
		}
		var records [][]uint16
		if records, err = h.HandleReadFileRecords(ctx, &gomodbus.ReadFileRecordsRequest{UnitID: unit, Requests: subs}); err == nil {
			if len(records) != len(subs) {
				t.Fatalf("unit %d: %d records for %d sub-requests", unit, len(records), len(subs))
			}
			body := []byte{}
			for i, rec := range records {
				if len(rec) != int(subs[i].RecordLength) {
					t.Fatalf("unit %d: record %d of %d registers, asked for %d", unit, i, len(rec), subs[i].RecordLength)
				}
				body = append(body, byte(1+2*len(rec)), 0x06)
				for _, v := range rec {
					body = be(body, v)
				}
			}
			same(append([]byte{0x14, byte(len(body))}, body...))
		}
	case 0x18:
		if len(req) != 3 {
			return
		}
		var values []uint16
		if values, err = h.HandleReadFIFOQueue(ctx, &gomodbus.FIFOQueueRequest{UnitID: unit, Addr: binary.BigEndian.Uint16(req[1:])}); err == nil {
			out := be(be([]byte{0x18}, uint16(2+2*len(values))), uint16(len(values)))
			for _, v := range values {
				out = be(out, v)
			}
			same(out)
		}
	case 0x2B:
		var r *gomodbus.DeviceIdentificationResponse
		if r, err = h.HandleDeviceIdentification(ctx, &gomodbus.DeviceIdentificationRequest{UnitID: unit, MEIType: 0x0E}); err == nil {
			if len(r.Objects) == 0 {
				t.Fatalf("unit %d: FC43 answered without objects", unit)
			}
			for i, o := range r.Objects {
				if len(o.Value) > 255 {
					t.Fatalf("unit %d: FC43 object %d of %d bytes", unit, o.ID, len(o.Value))
				}
				if i > 0 && r.Objects[i-1].ID >= o.ID {
					t.Fatalf("unit %d: FC43 objects out of order: %v", unit, r.Objects)
				}
			}
		}
	default:
		return
	}
	known := []error{gomodbus.ErrIllegalFunction, gomodbus.ErrIllegalDataAddress, gomodbus.ErrIllegalDataValue,
		gomodbus.ErrServerDeviceFailure, gomodbus.ErrBadUnitID, gomodbus.ErrNoResponse}
	if err != nil && !slices.ContainsFunc(known, func(k error) bool { return errors.Is(err, k) }) {
		t.Fatalf("unit %d % X: %v is not an answer of the server", unit, req, err)
	}
}

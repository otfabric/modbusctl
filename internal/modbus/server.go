// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/replay"
)

// Missing says how the server answers a request the capture holds nothing for.
type Missing int

const (
	// MissingException answers like a device would: Illegal Function for a
	// function never recorded, Illegal Data Address otherwise.
	MissingException Missing = iota
	// MissingZero answers unrecorded coils and registers with zeros.
	MissingZero
	// MissingSilence does not answer: the client times out.
	MissingSilence
)

// ParseMissing parses the --missing flag: exception (default), zero or silence.
func ParseMissing(s string) (Missing, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "exception":
		return MissingException, nil
	case "zero":
		return MissingZero, nil
	case "silence":
		return MissingSilence, nil
	}
	return 0, fmt.Errorf("invalid --missing %q: use exception, zero or silence", s)
}

// ParseBeforeFirst parses the --before-first flag: hold (default) or absent.
func ParseBeforeFirst(s string) (replay.BeforeFirst, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "hold":
		return replay.HoldFirst, nil
	case "absent":
		return replay.AbsentUntilSampled, nil
	}
	return 0, fmt.Errorf("invalid --before-first %q: use hold or absent", s)
}

// ParseSpeed parses the --speed flag: a positive factor, 1 by default.
func ParseSpeed(s string) (float64, error) {
	if strings.TrimSpace(s) == "" {
		return 1, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid --speed %q: use a positive factor such as 1, 0.1 or 60", s)
	}
	return v, nil
}

// ParseOffset parses a time since the capture start: a Go duration such as 90s
// or 1m30s. Empty is 0.
func ParseOffset(flag, s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid --%s %q: use a duration since the capture start, such as 90s or 1m30s", flag, s)
	}
	return d, nil
}

// captureHandler serves the targets of a capture, one per unit id, as of the
// replay time now() returns: FC01–04 per address, FC43 device identification
// per object, and FC07, FC08, FC11, FC12, FC17, FC20 and FC24 from the
// response recorded for exactly the request, through the go-modbus optional
// handlers. It answers reads only: go-modbus answers every other function
// code, writes and vendor or unknown functions included, with Illegal
// Function. It is safe for concurrent
// use: the devices are immutable and the counters atomic.
type captureHandler struct {
	devices map[uint8]*replay.Device
	now     func() capture.Ticks
	missing Missing
	stats   *serveStats
	// debug, when set, receives a line per request the capture holds nothing for.
	debug   io.Writer
	debugMu sync.Mutex
}

// serveStats counts what a server did: requests and their outcomes come from
// go-modbus ServerMetrics, unrecorded requests from the handler.
type serveStats struct {
	requests, answered, exceptions, silent, unrecorded atomic.Int64
}

func (s *serveStats) OnRequest(uint8, modbus.FunctionCode) { s.requests.Add(1) }

func (s *serveStats) OnResponse(uint8, modbus.FunctionCode, time.Duration) { s.answered.Add(1) }

func (s *serveStats) OnError(_ uint8, _ modbus.FunctionCode, _ time.Duration, err error) {
	if errors.Is(err, modbus.ErrNoResponse) {
		s.silent.Add(1)
		return
	}
	s.exceptions.Add(1)
}

// summary describes the counters in one line.
func (s *serveStats) summary() string {
	return fmt.Sprintf("📊 Served %d requests: %d answered, %d with an exception, %d left unanswered; %d asked for data the capture does not hold",
		s.requests.Load(), s.answered.Load(), s.exceptions.Load(), s.silent.Load(), s.unrecorded.Load())
}

func (h *captureHandler) unrecorded(unit, fc uint8, addr, qty uint16) {
	if h.stats != nil {
		h.stats.unrecorded.Add(1)
	}
	if h.debug != nil {
		h.debugMu.Lock()
		if fc >= 1 && fc <= 4 {
			_, _ = fmt.Fprintf(h.debug, "DEBUG [serve] unit %d FC%02d %d x%d: not in the capture\n", unit, fc, addr, qty)
		} else {
			_, _ = fmt.Fprintf(h.debug, "DEBUG [serve] unit %d FC%02d: not in the capture\n", unit, fc)
		}
		h.debugMu.Unlock()
	}
}

func (h *captureHandler) device(unit uint8) (*replay.Device, error) {
	if d := h.devices[unit]; d != nil {
		return d, nil
	}
	return nil, modbus.ErrBadUnitID
}

// absent answers a request the capture holds nothing for.
func (h *captureHandler) absent(d *replay.Device, fc uint8, qty uint16) ([]uint16, error) {
	switch h.missing {
	case MissingZero:
		if fc >= 1 && fc <= 4 {
			return make([]uint16, qty), nil
		}
	case MissingSilence:
		return nil, modbus.ErrNoResponse
	case MissingException:
	}
	if !d.HasFunction(fc) {
		return nil, modbus.ErrIllegalFunction
	}
	return nil, modbus.ErrIllegalDataAddress
}

func (h *captureHandler) values(unit, fc uint8, addr, qty uint16) ([]uint16, error) {
	d, err := h.device(unit)
	if err != nil {
		return nil, err
	}
	t := h.now()
	if vals, ok := d.Values(t, fc, addr, qty); ok {
		return vals, nil
	}
	h.unrecorded(unit, fc, addr, qty)
	if h.missing == MissingZero && d.HasFunction(fc) {
		// Recorded addresses keep their values; the others read as zero.
		vals, _ := d.Lookup(t, fc, addr, qty)
		if vals != nil {
			return vals, nil
		}
	}
	return h.absent(d, fc, qty)
}

func toBools(vals []uint16, err error) ([]bool, error) {
	if err != nil {
		return nil, err
	}
	out := make([]bool, len(vals))
	for i, v := range vals {
		out[i] = v != 0
	}
	return out, nil
}

func (h *captureHandler) HandleCoils(_ context.Context, req *modbus.CoilsRequest) ([]bool, error) {
	if req.IsWrite {
		return nil, modbus.ErrIllegalFunction // a replayed device is read-only
	}
	return toBools(h.values(req.UnitID, 1, req.Addr, req.Quantity))
}

func (h *captureHandler) HandleDiscreteInputs(_ context.Context, req *modbus.DiscreteInputsRequest) ([]bool, error) {
	return toBools(h.values(req.UnitID, 2, req.Addr, req.Quantity))
}

func (h *captureHandler) HandleHoldingRegisters(_ context.Context, req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	if req.IsWrite {
		return nil, modbus.ErrIllegalFunction
	}
	return h.values(req.UnitID, 3, req.Addr, req.Quantity)
}

func (h *captureHandler) HandleInputRegisters(_ context.Context, req *modbus.InputRegistersRequest) ([]uint16, error) {
	return h.values(req.UnitID, 4, req.Addr, req.Quantity)
}

// response returns the recorded response PDU to exactly req at the replay
// time, when it is a response to req's function holding at least n bytes
// after the function code; otherwise it answers as for an unrecorded request.
func (h *captureHandler) response(unit uint8, req []byte, n int) ([]byte, error) {
	d, err := h.device(unit)
	if err != nil {
		return nil, err
	}
	fc := req[0]
	resp, ok := d.Response(h.now(), req)
	if ok && len(resp) >= 1+n && resp[0] == fc {
		return resp, nil
	}
	h.unrecorded(unit, fc, 0, 0)
	_, err = h.absent(d, fc, 0)
	return nil, err
}

func (h *captureHandler) HandleExceptionStatus(_ context.Context, req *modbus.ExceptionStatusRequest) (uint8, error) {
	r, err := h.response(req.UnitID, []byte{0x07}, 1)
	if err != nil {
		return 0, err
	}
	return r[1], nil
}

func (h *captureHandler) HandleCommEventCounter(_ context.Context, req *modbus.CommEventCounterRequest) (*modbus.CommEventCounterResponse, error) {
	r, err := h.response(req.UnitID, []byte{0x0B}, 4)
	if err != nil {
		return nil, err
	}
	return &modbus.CommEventCounterResponse{Status: binary.BigEndian.Uint16(r[1:]), EventCount: binary.BigEndian.Uint16(r[3:])}, nil
}

func (h *captureHandler) HandleCommEventLog(_ context.Context, req *modbus.CommEventLogRequest) (*modbus.CommEventLogResponse, error) {
	r, err := h.response(req.UnitID, []byte{0x0C}, 7)
	if err != nil {
		return nil, err
	}
	// r = FC, byte count, status, event count, message count, events.
	if int(r[1]) != len(r)-2 {
		return nil, modbus.ErrServerDeviceFailure
	}
	return &modbus.CommEventLogResponse{Status: binary.BigEndian.Uint16(r[2:]), EventCount: binary.BigEndian.Uint16(r[4:]),
		MessageCount: binary.BigEndian.Uint16(r[6:]), Events: append([]byte(nil), r[8:]...)}, nil
}

// HandleDiagnostics serves FC08 sub-functions that return data (see
// [capfile.IsDataDiagnostic]); the others change device state and are
// refused with Illegal Function. The server echoes the sub-function in front
// of the recorded data.
func (h *captureHandler) HandleDiagnostics(_ context.Context, req *modbus.DiagnosticsRequest) ([]byte, error) {
	sub := uint16(req.SubFunction)
	if sub > 0xFF || !capfile.IsDataDiagnostic(uint8(sub)) {
		return nil, modbus.ErrIllegalFunction
	}
	r, err := h.response(req.UnitID, append([]byte{0x08, byte(sub >> 8), byte(sub)}, req.Data...), 2)
	if err != nil {
		return nil, err
	}
	if binary.BigEndian.Uint16(r[1:]) != sub {
		return nil, modbus.ErrServerDeviceFailure
	}
	return append([]byte(nil), r[3:]...), nil
}

// HandleReportServerID serves FC17 with the recorded data after the byte
// count; the server adds the byte count.
func (h *captureHandler) HandleReportServerID(_ context.Context, req *modbus.ReportServerIDRequest) ([]byte, error) {
	r, err := h.response(req.UnitID, []byte{0x11}, 1)
	if err != nil {
		return nil, err
	}
	if int(r[1]) != len(r)-2 {
		return nil, modbus.ErrServerDeviceFailure
	}
	return append([]byte(nil), r[2:]...), nil
}

// HandleReadFileRecords serves FC20 with the records of the response
// recorded for exactly these sub-requests.
func (h *captureHandler) HandleReadFileRecords(_ context.Context, req *modbus.ReadFileRecordsRequest) ([][]uint16, error) {
	pdu := []byte{0x14, byte(7 * len(req.Requests))}
	for _, s := range req.Requests {
		pdu = append(pdu, 0x06, byte(s.FileNumber>>8), byte(s.FileNumber), byte(s.RecordNumber>>8), byte(s.RecordNumber),
			byte(s.RecordLength>>8), byte(s.RecordLength))
	}
	r, err := h.response(req.UnitID, pdu, 1)
	if err != nil {
		return nil, err
	}
	// r = FC, data length, then per sub-request: length, reference type 6, registers.
	if int(r[1]) != len(r)-2 {
		return nil, modbus.ErrServerDeviceFailure
	}
	records, p := make([][]uint16, len(req.Requests)), 2
	for i, s := range req.Requests {
		if p+2 > len(r) || r[p+1] != 0x06 || int(r[p]) != 1+2*int(s.RecordLength) || p+1+int(r[p]) > len(r) {
			return nil, modbus.ErrServerDeviceFailure
		}
		records[i] = make([]uint16, s.RecordLength)
		for k := range records[i] {
			records[i][k] = binary.BigEndian.Uint16(r[p+2+2*k:])
		}
		p += 1 + int(r[p])
	}
	if p != len(r) {
		return nil, modbus.ErrServerDeviceFailure
	}
	return records, nil
}

// HandleReadFIFOQueue serves FC24 with the recorded queue.
func (h *captureHandler) HandleReadFIFOQueue(_ context.Context, req *modbus.FIFOQueueRequest) ([]uint16, error) {
	r, err := h.response(req.UnitID, []byte{0x18, byte(req.Addr >> 8), byte(req.Addr)}, 4)
	if err != nil {
		return nil, err
	}
	// r = FC, byte count, FIFO count, values.
	count := int(binary.BigEndian.Uint16(r[3:]))
	if int(binary.BigEndian.Uint16(r[1:])) != len(r)-3 || len(r) != 5+2*count {
		return nil, modbus.ErrServerDeviceFailure
	}
	values := make([]uint16, count)
	for i := range values {
		values[i] = binary.BigEndian.Uint16(r[5+2*i:])
	}
	return values, nil
}

func (h *captureHandler) HandleDeviceIdentification(_ context.Context, req *modbus.DeviceIdentificationRequest) (*modbus.DeviceIdentificationResponse, error) {
	d, err := h.device(req.UnitID)
	if err != nil {
		return nil, err
	}
	conf, objs, ok := d.Identification(h.now())
	if !ok {
		h.unrecorded(req.UnitID, 0x2B, 0, 0)
		_, err := h.absent(d, 0x2B, 0)
		return nil, err
	}
	res := &modbus.DeviceIdentificationResponse{ConformityLevel: conf}
	for _, o := range objs {
		res.Objects = append(res.Objects, modbus.DeviceIdentificationObject{ID: modbus.DeviceIDObjectID(o.ID), Value: string(o.Value)})
	}
	return res, nil
}

// serveTargets loads the capture and builds one device per served target,
// keyed by the unit id it answers on.
func serveTargets(input string, target uint32, unit uint8, opts replay.Options) (*capfile.Loaded, map[uint8]*replay.Device, error) {
	l, err := capfile.Load(input)
	if err != nil {
		return nil, nil, capfile.WrapError(err)
	}
	var ids []uint32
	if target != 0 {
		if _, ok := l.Target(target); !ok {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidInput,
				fmt.Sprintf("the capture has no target %d; its targets are: %s", target, l.ListTargets()), nil)
		}
		ids = []uint32{target}
	} else {
		for _, t := range l.Targets {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, "the capture has no targets", nil)
	}
	if unit != 0 && len(ids) > 1 {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination,
			fmt.Sprintf("--unit needs a single target: choose one with --target (%s)", l.ListTargets()), nil)
	}
	devices := map[uint8]*replay.Device{}
	for _, id := range ids {
		d, err := replay.Build(l.Capture, id, l.End(), opts)
		if err != nil {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
		}
		u := d.Target().Unit
		if unit != 0 {
			u = unit
		}
		if other := devices[u]; other != nil {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination,
				fmt.Sprintf("targets %d and %d both answer on unit %d: serve one with --target", other.Target().ID, id, u), nil)
		}
		devices[u] = d
	}
	return l, devices, nil
}

func describeDevices(w io.Writer, devices map[uint8]*replay.Device) {
	units := make([]int, 0, len(devices))
	for u := range devices {
		units = append(units, int(u))
	}
	sort.Ints(units)
	for _, u := range units {
		d := devices[uint8(u)]
		t := d.Target()
		label := ""
		if t.Label != "" {
			label = fmt.Sprintf(" %q", t.Label)
		}
		_, _ = fmt.Fprintf(w, "🧱 Unit %d: target %d%s, %d samples\n", u, t.ID, label, d.Samples())
	}
}

// ServerConfig configures the Modbus TCP listener.
type ServerConfig struct {
	// Listen is the address to listen on; empty is every interface.
	Listen string
	Port   uint16
	// MaxClients limits concurrent client connections; 0 is the go-modbus
	// default (10).
	MaxClients uint16
	// TLSCert, when set, serves Modbus/TCP Security (tcp+tls) instead of
	// plain Modbus TCP: the server presents it and requires a client
	// certificate signed by TLSClientCAs.
	TLSCert      *tls.Certificate
	TLSClientCAs *x509.CertPool
	Debug        bool
}

// serve runs the handler on a Modbus TCP listener until ctx ends, then prints
// what it served.
func serve(ctx context.Context, h *captureHandler, cfg ServerConfig, progress io.Writer) error {
	h.stats = &serveStats{}
	if cfg.Debug {
		h.debug = progress
	}
	err := newModbusServer(ctx, h, h.stats, cfg, progress)
	if h.stats.requests.Load() > 0 || errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(progress, h.stats.summary())
	}
	return err
}

func newModbusServer(ctx context.Context, handler modbus.RequestHandler, metrics modbus.ServerMetrics, cfg ServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	logger := modbus.NopLogger()
	if cfg.Debug {
		logger = modbus.NewStdLogger(nil)
	}
	host := cfg.Listen
	if host == "" {
		host = "0.0.0.0"
	}
	scheme, what := "tcp", "Modbus TCP"
	if cfg.TLSCert != nil {
		scheme, what = "tcp+tls", "Modbus TCP Security (tcp+tls, client certificates required)"
	}
	server, err := modbus.NewServer(&modbus.ServerConfig{
		URL:           scheme + "://" + net.JoinHostPort(host, strconv.Itoa(int(cfg.Port))),
		Timeout:       0,
		MaxClients:    uint(cfg.MaxClients),
		Logger:        logger,
		Metrics:       metrics,
		TLSServerCert: cfg.TLSCert,
		TLSClientCAs:  cfg.TLSClientCAs,
	}, handler)
	if err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "failed to start modbus server: "+err.Error(), err)
	}
	if err := server.Start(); err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "modbus server failed: "+err.Error(), err)
	}
	// Printed once the listener is bound: from here on clients reach this server.
	if cfg.Listen == "" {
		_, _ = fmt.Fprintf(progress, "🚀 Serving %s on port %d\n", what, cfg.Port)
	} else {
		_, _ = fmt.Fprintf(progress, "🚀 Serving %s on %s port %d\n", what, cfg.Listen, cfg.Port)
	}
	// Block until context canceled (e.g. SIGINT/SIGTERM via root Execute).
	<-ctx.Done()
	// Close listener and client sockets (go-modbus); ignore error—return cancel reason for CLI exit handling.
	_ = server.Stop()
	return ctx.Err()
}

// ServeStatic serves the values of a capture as of one moment (by default
// its end: the last values recorded).
func ServeStatic(ctx context.Context, cfg config.StaticServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	missing, err := ParseMissing(cfg.Missing)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	at, err := ParseOffset("at", cfg.At)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	cert, clientCAs, err := cfg.Load()
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	l, devices, err := serveTargets(cfg.InputFile, cfg.Target, cfg.Unit, replay.Options{})
	if err != nil {
		return err
	}
	t := l.End()
	when := "the last recorded values"
	if strings.TrimSpace(cfg.At) != "" {
		t = min(l.Header.Ticks(at), l.End())
		when = "the values at " + l.Header.Duration(t).String()
	}
	if l.Truncated {
		_, _ = fmt.Fprintf(progress, "⚠️ %s is truncated; serving what it holds\n", cfg.InputFile)
	}
	_, _ = fmt.Fprintf(progress, "✅ Static server ready on port %d with %s of %s\n", cfg.Port, when, cfg.InputFile)
	describeDevices(progress, devices)
	h := &captureHandler{devices: devices, now: func() capture.Ticks { return t }, missing: missing}
	return serve(ctx, h, ServerConfig{Listen: cfg.Listen, Port: cfg.Port, MaxClients: cfg.MaxClients, TLSCert: cert, TLSClientCAs: clientCAs, Debug: cfg.Debug}, progress)
}

// ServeReplay replays a capture: the served values follow the recorded ones
// at the chosen speed, looping as configured.
func ServeReplay(ctx context.Context, cfg config.ReplayServerConfig, progress io.Writer) error {
	if progress == nil {
		progress = io.Discard
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	missing, err := ParseMissing(cfg.Missing)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	before, err := ParseBeforeFirst(cfg.BeforeFirst)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	speed, err := ParseSpeed(cfg.Speed)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	from, err := ParseOffset("from", cfg.From)
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	cert, clientCAs, err := cfg.Load()
	if err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}
	l, devices, err := serveTargets(cfg.InputFile, cfg.Target, cfg.Unit, replay.Options{BeforeFirst: before})
	if err != nil {
		return err
	}
	clock := replay.Clock{TickNS: l.Header.TickNS, Speed: speed, From: min(l.Header.Ticks(from), l.End()), To: l.End(), Loops: int(cfg.Loops)}
	if l.Truncated {
		_, _ = fmt.Fprintf(progress, "⚠️ %s is truncated; replaying what it holds\n", cfg.InputFile)
	}
	loops := "forever"
	if cfg.Loops > 0 {
		loops = fmt.Sprintf("%d time(s), then the last values stay", cfg.Loops)
	}
	_, _ = fmt.Fprintf(progress, "✅ Replay server ready on port %d: %s of %s at %gx, looping %s\n",
		cfg.Port, l.Header.Duration(clock.To-clock.From), cfg.InputFile, speed, loops)
	describeDevices(progress, devices)
	start := time.Now()
	h := &captureHandler{devices: devices, now: func() capture.Ticks { return clock.At(time.Since(start)) }, missing: missing}
	return serve(ctx, h, ServerConfig{Listen: cfg.Listen, Port: cfg.Port, MaxClients: cfg.MaxClients, TLSCert: cert, TLSClientCAs: clientCAs, Debug: cfg.Debug}, progress)
}

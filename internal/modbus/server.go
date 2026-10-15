package modbus

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// per object, and every other function with the recorded response PDU
// (go-modbus RawRequestHandler). It answers reads only. It is safe for concurrent
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

// isWrite reports whether fc changes device state: FC21 Write File Record,
// FC22 Mask Write Register and FC23 Read/Write Multiple Registers reach
// HandleRawRequest; FC05, FC06, FC15 and FC16 arrive with IsWrite set.
func isWrite(fc uint8) bool { return fc == 0x15 || fc == 0x16 || fc == 0x17 }

// HandleRawRequest answers every function go-modbus does not decode for the
// handler (FC07, FC08, FC11, FC12, FC17, FC20, FC24, FC43 other than device
// identification, vendor functions) with the response recorded for exactly
// this request PDU, byte for byte.
func (h *captureHandler) HandleRawRequest(_ context.Context, req *modbus.RawRequest) ([]byte, error) {
	fc := uint8(req.FunctionCode)
	if isWrite(fc) {
		return nil, modbus.ErrIllegalFunction // a replayed device is read-only
	}
	d, err := h.device(req.UnitID)
	if err != nil {
		return nil, err
	}
	resp, ok := d.Response(h.now(), append([]byte{fc}, req.Payload...))
	if ok && len(resp) >= 1 && resp[0] == fc {
		return resp[1:], nil
	}
	h.unrecorded(req.UnitID, fc, 0, 0)
	_, err = h.absent(d, fc, 0)
	return nil, err
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
	Port uint16
	// MaxClients limits concurrent client connections; 0 is the go-modbus
	// default (10).
	MaxClients uint16
	Debug      bool
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
	server, err := modbus.NewServer(&modbus.ServerConfig{
		URL:        fmt.Sprintf("tcp://0.0.0.0:%d", cfg.Port),
		Timeout:    0,
		MaxClients: uint(cfg.MaxClients),
		Logger:     logger,
		Metrics:    metrics,
	}, handler)
	if err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "failed to start modbus server: "+err.Error(), err)
	}
	_, _ = fmt.Fprintf(progress, "🚀 Serving Modbus TCP on port %d\n", cfg.Port)
	if err := server.Start(); err != nil {
		return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "modbus server failed: "+err.Error(), err)
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
	return serve(ctx, h, ServerConfig{Port: cfg.Port, MaxClients: cfg.MaxClients, Debug: cfg.Debug}, progress)
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
	return serve(ctx, h, ServerConfig{Port: cfg.Port, MaxClients: cfg.MaxClients, Debug: cfg.Debug}, progress)
}

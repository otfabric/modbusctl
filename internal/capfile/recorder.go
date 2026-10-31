// SPDX-License-Identifier: MIT

// Package capfile connects modbusctl to the MBCAP capture format of
// github.com/otfabric/go-modbus-capture: a Recorder that writes what a device
// answers through the go-modbus PDU tap, a loader for capture files in any
// MBCAP encoding, and an adapter to the register-block records the decode,
// strings, frequencies and extract commands work on.
package capfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/mbcap"
	"github.com/otfabric/modbusctl/internal/config"
)

// Producer names modbusctl in the header of the captures it writes. It is set
// from the build metadata.
var Producer = "modbusctl"

// flushEvery bounds how long recorded samples stay in memory before they are
// flushed to the file, so that a capture interrupted by a crash or a power
// cut loses at most this much.
const flushEvery = 10 * time.Second

// RecorderOptions configure a Recorder.
type RecorderOptions struct {
	// Exceptions records exception responses too (default: only answers
	// with data).
	Exceptions bool
	// Tags are added to the capture header.
	Tags map[string]string
}

// Recorder writes an MBCAP capture of the requests a go-modbus client sends.
// Install Tap as the client's Config.PDUTap; wrap each attempt in Begin and
// Commit. Exchanges are written only for attempts that Commit accepts, so a
// failed or retried attempt leaves no sample. A Recorder is safe for
// concurrent use.
type Recorder struct {
	mu        sync.Mutex
	path      string
	f         *os.File
	w         *mbcap.Writer
	h         capture.Header
	start     time.Time
	lastFlush time.Time
	opts      RecorderOptions

	pending map[uint64][]modbus.PDUExchange
	nextTok uint64
	targets map[uint32]bool
	streams map[uint32]bool
	samples int
	closed  bool
}

// Create opens path and writes the capture header. The file is written as the
// recording goes: it stays readable up to its last flushed block if the
// recording is interrupted.
func Create(path string, opts RecorderOptions) (*Recorder, error) {
	start := time.Now()
	h := capture.Header{
		TickNS:     capture.DefaultTickNS,
		WallStart:  start.UTC(),
		Exceptions: opts.Exceptions,
		Producer:   Producer,
		Tags:       opts.Tags,
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w, err := mbcap.NewWriter(f, h, nil)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Recorder{path: path, f: f, w: w, h: h, start: start, lastFlush: start, opts: opts,
		pending: map[uint64][]modbus.PDUExchange{}, targets: map[uint32]bool{}, streams: map[uint32]bool{}}, nil
}

// Path returns the file being written.
func (r *Recorder) Path() string { return r.path }

// Samples returns the number of samples written so far.
func (r *Recorder) Samples() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.samples
}

// AddTarget defines a target, once; later calls with the same id are ignored.
func (r *Recorder) AddTarget(t capture.Target) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.targets[t.ID] {
		return nil
	}
	if err := r.w.WriteTarget(t); err != nil {
		return err
	}
	r.targets[t.ID] = true
	return nil
}

// AddStream defines a stream, once; later calls with the same id are ignored.
func (r *Recorder) AddStream(s capture.Stream) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streams[s.ID] {
		return nil
	}
	if err := r.w.WriteStream(s); err != nil {
		return err
	}
	r.streams[s.ID] = true
	return nil
}

type tokenKey struct{}

// Begin marks the start of one request attempt. Pass the returned context to
// the client method, then call Commit with the token.
func (r *Recorder) Begin(ctx context.Context) (context.Context, uint64) {
	r.mu.Lock()
	r.nextTok++
	tok := r.nextTok
	r.mu.Unlock()
	return context.WithValue(ctx, tokenKey{}, tok), tok
}

// Tap returns the PDU tap to install in the client configuration.
func (r *Recorder) Tap() modbus.PDUTap { return modbus.PDUTapFunc(r.onPDU) }

func (r *Recorder) onPDU(ctx context.Context, ex modbus.PDUExchange) {
	tok, ok := ctx.Value(tokenKey{}).(uint64)
	if !ok {
		return // not a recorded call
	}
	r.mu.Lock()
	r.pending[tok] = append(r.pending[tok], ex)
	r.mu.Unlock()
}

// Commit writes the exchanges of attempt tok as samples of stream when the
// attempt succeeded (err == nil), or when it ended in a Modbus exception and
// the recorder records exceptions. Otherwise it drops them.
func (r *Recorder) Commit(tok uint64, stream uint32, err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exs := r.pending[tok]
	delete(r.pending, tok)
	if r.closed || len(exs) == 0 {
		return nil
	}
	var exc *modbus.ExceptionError
	keep := err == nil || r.opts.Exceptions && errors.As(err, &exc)
	if !keep {
		return nil
	}
	for _, ex := range exs {
		s := capture.Sample{Stream: stream, At: r.h.Ticks(ex.Received.Sub(r.start)), Request: ex.Request, Response: ex.Response}
		if capture.IsWriteFunction(s.Function()) {
			continue // captures hold reads only
		}
		if !IsSpecRead(s.Request) {
			return fmt.Errorf("%w: not recording function code %d (request % X)", ErrUnsupportedFunction, s.Function(), s.Request)
		}
		if werr := r.w.WriteSample(s); werr != nil {
			return fmt.Errorf("writing capture %s: %w", r.path, werr)
		}
		r.samples++
	}
	if time.Since(r.lastFlush) >= flushEvery {
		r.lastFlush = time.Now()
		return r.w.Flush()
	}
	return nil
}

// Close completes the capture with its end (now) and closes the file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.w.Close(capture.End{Known: true, At: r.h.Ticks(time.Since(r.start))})
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	return err
}

// TargetFor describes the device behind modbusURL as a capture target. The
// URL itself is kept in the "url" tag, as the transports the format names do
// not cover every go-modbus scheme.
func TargetFor(id uint32, modbusURL string, unit uint8, serial config.SerialConfig, label string) capture.Target {
	t := capture.Target{ID: id, Label: label, Unit: unit, Tags: map[string]string{"url": modbusURL}}
	scheme := modbusURL
	if i := strings.Index(modbusURL, "://"); i >= 0 {
		scheme = modbusURL[:i]
	}
	switch scheme {
	case "tcp":
		t.Transport = capture.TransportTCP
	case "tcp+tls":
		t.Transport = capture.TransportTLS
	case "udp":
		t.Transport = capture.TransportUDP
	case "rtuovertcp":
		t.Transport = capture.TransportRTUOverTCP
	case "rtu":
		t.Transport = capture.TransportRTU
	case "ascii":
		t.Transport = capture.TransportASCII
	default:
		t.Transport = capture.TransportUnknown
	}
	host, port := config.CaptureEndpoint(modbusURL)
	if config.IsSerialURL(modbusURL) {
		parity, _ := config.NormalizeParity(serial.Parity)
		p, _ := capture.ParseParity(parity)
		t.Serial = capture.Serial{Device: host, Baud: uint32(serial.Baud), Parity: p,
			DataBits: uint8(serial.DataBits), StopBits: uint8(serial.StopBits)}
	} else {
		t.Host, t.Port = host, port
	}
	return t
}

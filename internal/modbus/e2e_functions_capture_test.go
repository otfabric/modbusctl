package modbus

// End-to-end tests of the functions the servers answer through the go-modbus
// RawRequestHandler (FC08 Diagnostics, FC17 Report Server ID, FC20 Read File
// Record, FC24 Read FIFO Queue): record them from a go-modbus server, then
// serve the recording back, static and replayed, and check that each answer is
// the recorded response PDU byte for byte. Vendor functions and refused writes
// are tested on the handler itself.

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/replay"
	"github.com/stretchr/testify/require"
)

// e2eFunctionsDevice is a go-modbus server handler for FC08, FC17, FC20 and
// FC24. Its bus message count, FIFO queue and file record change with every
// answer, so successive samples of the same request differ.
type e2eFunctionsDevice struct {
	mu sync.Mutex
	n  uint16
}

func (d *e2eFunctionsDevice) next() uint16 {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.n++
	return d.n
}

func (d *e2eFunctionsDevice) HandleCoils(context.Context, *gomodbus.CoilsRequest) ([]bool, error) {
	return nil, gomodbus.ErrIllegalFunction
}

func (d *e2eFunctionsDevice) HandleDiscreteInputs(context.Context, *gomodbus.DiscreteInputsRequest) ([]bool, error) {
	return nil, gomodbus.ErrIllegalFunction
}

func (d *e2eFunctionsDevice) HandleHoldingRegisters(context.Context, *gomodbus.HoldingRegistersRequest) ([]uint16, error) {
	return nil, gomodbus.ErrIllegalFunction
}

func (d *e2eFunctionsDevice) HandleInputRegisters(context.Context, *gomodbus.InputRegistersRequest) ([]uint16, error) {
	return nil, gomodbus.ErrIllegalFunction
}

func (d *e2eFunctionsDevice) HandleDiagnostics(_ context.Context, req *gomodbus.DiagnosticsRequest) ([]byte, error) {
	if req.SubFunction == gomodbus.DiagReturnBusMessageCount {
		n := d.next()
		return []byte{byte(n >> 8), byte(n)}, nil
	}
	return req.Data, nil // Return Query Data echoes; the test asks nothing else
}

func (d *e2eFunctionsDevice) HandleReportServerID(context.Context, *gomodbus.ReportServerIDRequest) ([]byte, error) {
	return []byte{0x2A, 'm', 'b', 'c', 't', 'l', 0xFF}, nil
}

func (d *e2eFunctionsDevice) HandleReadFileRecords(_ context.Context, req *gomodbus.ReadFileRecordsRequest) ([][]uint16, error) {
	n := d.next()
	out := make([][]uint16, len(req.Requests))
	for i, r := range req.Requests {
		out[i] = make([]uint16, r.RecordLength)
		for k := range out[i] {
			out[i][k] = r.FileNumber<<12 | r.RecordNumber<<4 | uint16(k) + n
		}
	}
	return out, nil
}

func (d *e2eFunctionsDevice) HandleReadFIFOQueue(_ context.Context, req *gomodbus.FIFOQueueRequest) ([]uint16, error) {
	n := d.next()
	return []uint16{req.Addr, n, n * 3}[:1+n%3], nil
}

// e2eFunctionsServe serves h on a free TCP port with go-modbus and returns its URL.
func e2eFunctionsServe(t *testing.T, h gomodbus.RequestHandler) string {
	t.Helper()
	url := fmt.Sprintf("tcp://127.0.0.1:%d", collFreePort(t))
	srv, err := gomodbus.NewServer(&gomodbus.ServerConfig{URL: url, Logger: gomodbus.NopLogger()}, h)
	require.NoError(t, err)
	require.NoError(t, srv.Start())
	t.Cleanup(func() { _ = srv.Stop() })
	return url
}

// e2eFunctionsClient opens a client to url; tap, when set, sees every exchange.
func e2eFunctionsClient(t *testing.T, url string, tap gomodbus.PDUTap) *gomodbus.Client {
	t.Helper()
	c, err := gomodbus.New(gomodbus.Config{URL: url, Timeout: 2 * time.Second, Logger: gomodbus.NopLogger(), PDUTap: tap})
	require.NoError(t, err)
	require.NoError(t, c.Open())
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// e2eFunctionsCalls are the requests recorded and replayed, one per function
// (and two FC08 sub-functions).
var e2eFunctionsCalls = []struct {
	name string
	call func(ctx context.Context, c *gomodbus.Client, unit uint8) error
}{
	{"FC08 return query data", func(ctx context.Context, c *gomodbus.Client, unit uint8) error {
		_, err := c.Diagnostics(ctx, unit, gomodbus.DiagReturnQueryData, []byte{0xA5, 0x37})
		return err
	}},
	{"FC08 bus message count", func(ctx context.Context, c *gomodbus.Client, unit uint8) error {
		_, err := c.Diagnostics(ctx, unit, gomodbus.DiagReturnBusMessageCount, []byte{0, 0})
		return err
	}},
	{"FC17", func(ctx context.Context, c *gomodbus.Client, unit uint8) error {
		_, err := c.ReportServerID(ctx, unit)
		return err
	}},
	{"FC20", func(ctx context.Context, c *gomodbus.Client, unit uint8) error {
		_, err := c.ReadFileRecords(ctx, unit, []gomodbus.FileRecordRequest{
			{FileNumber: 1, RecordNumber: 2, RecordLength: 3}, {FileNumber: 4, RecordNumber: 0, RecordLength: 1}})
		return err
	}},
	{"FC24", func(ctx context.Context, c *gomodbus.Client, unit uint8) error {
		_, err := c.ReadFIFOQueue(ctx, unit, 0x04D2)
		return err
	}},
}

// e2eFunctionsRecord records rounds of e2eFunctionsCalls from a go-modbus
// server on unit 1, gap apart, and returns the capture path and the responses
// recorded per request PDU, in order.
func e2eFunctionsRecord(t *testing.T, rounds int, gap time.Duration) (string, map[string][][]byte) {
	t.Helper()
	url := e2eFunctionsServe(t, &e2eFunctionsDevice{})
	path := filepath.Join(t.TempDir(), "functions.mbcap")
	rec, err := capfile.Create(path, capfile.RecorderOptions{})
	require.NoError(t, err)
	require.NoError(t, rec.AddTarget(capfile.TargetFor(1, url, 1, config.SerialConfig{}, "functions")))
	require.NoError(t, rec.AddStream(capture.Stream{ID: 1, Target: 1, Role: "poll"}))
	c := e2eFunctionsClient(t, url, rec.Tap())
	for r := range rounds {
		if r > 0 {
			time.Sleep(gap)
		}
		for _, f := range e2eFunctionsCalls {
			ctx, tok := rec.Begin(context.Background())
			err := f.call(ctx, c, 1)
			require.NoError(t, err, f.name)
			require.NoError(t, rec.Commit(tok, 1, err))
		}
	}
	require.NoError(t, rec.Close())

	l, err := capfile.Load(path)
	require.NoError(t, err)
	recorded := map[string][][]byte{}
	for _, s := range l.Samples {
		recorded[string(s.Request)] = append(recorded[string(s.Request)], s.Response)
	}
	require.Len(t, l.Samples, rounds*len(e2eFunctionsCalls), "one sample per request")
	for _, fc := range []byte{0x08, 0x11, 0x14, 0x18} {
		require.Truef(t, func() bool {
			for k := range recorded {
				if k[0] == fc {
					return true
				}
			}
			return false
		}(), "FC%d recorded", fc)
	}
	return path, recorded
}

// e2eFunctionsAsk sends every call to c (tapped by served) and returns the
// exchanges of this round.
func e2eFunctionsAsk(t *testing.T, c *gomodbus.Client, served *e2eExchanges) []gomodbus.PDUExchange {
	t.Helper()
	from := served.len()
	for _, f := range e2eFunctionsCalls {
		require.NoError(t, f.call(context.Background(), c, 1), f.name)
	}
	got := served.since(from)
	require.Len(t, got, len(e2eFunctionsCalls))
	return got
}

// e2eExchanges collects the exchanges a client tap sees.
type e2eExchanges struct {
	mu  sync.Mutex
	exs []gomodbus.PDUExchange
}

func (e *e2eExchanges) OnPDU(_ context.Context, ex gomodbus.PDUExchange) {
	e.mu.Lock()
	e.exs = append(e.exs, ex)
	e.mu.Unlock()
}

func (e *e2eExchanges) len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.exs)
}

func (e *e2eExchanges) since(i int) []gomodbus.PDUExchange {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]gomodbus.PDUExchange(nil), e.exs[i:]...)
}

func requireRecordedLast(t *testing.T, recorded map[string][][]byte, exs []gomodbus.PDUExchange) {
	t.Helper()
	for _, ex := range exs {
		resps := recorded[string(ex.Request)]
		require.NotEmptyf(t, resps, "request % X was recorded", ex.Request)
		require.Equalf(t, hex.EncodeToString(resps[len(resps)-1]), hex.EncodeToString(ex.Response),
			"request % X is answered with its last recorded response", ex.Request)
	}
}

func TestE2EFunctionsCapture_static(t *testing.T) {
	path, recorded := e2eFunctionsRecord(t, 3, 0)
	srv, progress := staticServer(t, config.StaticServerConfig{InputFile: path})
	var served e2eExchanges
	c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &served)
	for range 3 {
		requireRecordedLast(t, recorded, e2eFunctionsAsk(t, c, &served))
	}
	// A request the capture does not hold, of a recorded function.
	_, err := c.Diagnostics(context.Background(), 1, gomodbus.DiagReturnQueryData, []byte{0x12, 0x34})
	collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
	require.ErrorIs(t, srv.stop(t), context.Canceled)
	require.Contains(t, progress.String(), "📊 Served 16 requests: 15 answered, 1 with an exception, 0 left unanswered; 1 asked for data the capture does not hold")
}

func TestE2EFunctionsCapture_replay(t *testing.T) {
	const rounds, gap = 4, 50 * time.Millisecond
	path, recorded := e2eFunctionsRecord(t, rounds, gap)
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return ServeReplay(ctx, config.ReplayServerConfig{Port: port, InputFile: path, Speed: "1", Loops: 1}, nil)
	})
	var served e2eExchanges
	c := e2eFunctionsClient(t, fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), &served)

	// While the capture plays, every answer is one of the responses recorded
	// for that request, and the changing ones go through more than one.
	distinct := map[string]map[string]bool{}
	end := time.Now().Add(rounds * gap)
	for time.Now().Before(end) {
		for _, ex := range e2eFunctionsAsk(t, c, &served) {
			found := false
			for _, r := range recorded[string(ex.Request)] {
				found = found || bytes.Equal(r, ex.Response)
			}
			require.Truef(t, found, "request % X answered with % X, which was never recorded for it", ex.Request, ex.Response)
			if distinct[string(ex.Request)] == nil {
				distinct[string(ex.Request)] = map[string]bool{}
			}
			distinct[string(ex.Request)][string(ex.Response)] = true
		}
		time.Sleep(5 * time.Millisecond)
	}
	changed := 0
	for _, resps := range distinct {
		if len(resps) > 1 {
			changed++
		}
	}
	require.Positive(t, changed, "replay follows the recorded changes")

	// After the single loop the last recorded responses stay.
	time.Sleep(2 * gap)
	requireRecordedLast(t, recorded, e2eFunctionsAsk(t, c, &served))
	require.ErrorIs(t, srv.stop(t), context.Canceled)
}

// The raw handler answers vendor functions and FC43 MEI types other than
// device identification with the recorded response, refuses writes, and
// answers what the capture does not hold according to --missing.
func TestCaptureHandler_rawRequests(t *testing.T) {
	c := &capture.Capture{
		Header:  capture.Header{TickNS: 1000},
		Targets: []capture.Target{{ID: 1, Unit: 5}},
		Streams: []capture.Stream{{ID: 1, Target: 1}},
		Samples: []capture.Sample{
			{Stream: 1, At: 0, Request: []byte{0x41, 0x01, 0x02}, Response: []byte{0x41, 0xCA, 0xFE}},
			{Stream: 1, At: 10, Request: []byte{0x41, 0x01, 0x02}, Response: []byte{0x41, 0xBE, 0xEF}},
			{Stream: 1, At: 20, Request: []byte{0x2B, 0x0D, 0x00}, Response: []byte{0x2B, 0x0D, 0x00, 0x99}},
			{Stream: 1, At: 30, Request: []byte{0x07}, Response: []byte{0x07, 0x6D}},
		},
		End: capture.End{Known: true, At: 100},
	}
	d, err := replay.Build(c, 1, 100, replay.Options{})
	require.NoError(t, err)
	at := capture.Ticks(5)
	h := &captureHandler{devices: map[uint8]*replay.Device{5: d}, now: func() capture.Ticks { return at }}
	ask := func(fc uint8, payload ...byte) ([]byte, error) {
		return h.HandleRawRequest(context.Background(), &gomodbus.RawRequest{UnitID: 5, FunctionCode: gomodbus.FunctionCode(fc), Payload: payload})
	}

	got, err := ask(0x41, 0x01, 0x02)
	require.NoError(t, err)
	require.Equal(t, []byte{0xCA, 0xFE}, got, "the sample at T")
	at = 50
	got, err = ask(0x41, 0x01, 0x02)
	require.NoError(t, err)
	require.Equal(t, []byte{0xBE, 0xEF}, got, "the later sample")
	got, err = ask(0x2B, 0x0D, 0x00)
	require.NoError(t, err)
	require.Equal(t, []byte{0x0D, 0x00, 0x99}, got)

	for _, fc := range []uint8{0x15, 0x16, 0x17} {
		_, err = ask(fc, 0x00, 0x01)
		require.ErrorIsf(t, err, gomodbus.ErrIllegalFunction, "write FC%d", fc)
	}
	_, err = ask(0x41, 0x09)
	require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress, "recorded function, other request")
	_, err = ask(0x42)
	require.ErrorIs(t, err, gomodbus.ErrIllegalFunction, "function never recorded")
	_, err = h.HandleRawRequest(context.Background(), &gomodbus.RawRequest{UnitID: 6, FunctionCode: 0x41})
	require.ErrorIs(t, err, gomodbus.ErrBadUnitID)

	h.missing = MissingZero
	_, err = ask(0x41, 0x09)
	require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress, "zero applies to coils and registers only")
	h.missing = MissingSilence
	_, err = ask(0x41, 0x09)
	require.ErrorIs(t, err, gomodbus.ErrNoResponse)
}

package modbus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collWriteCapture writes c into a temp dir and returns its path.
func collWriteCapture(t *testing.T, c *capture.Capture) string {
	t.Helper()
	return capfiletest.Write(t, filepath.Join(t.TempDir(), "capture.mbcap"), c)
}

// collFreePort returns a TCP port that is free on every interface.
func collFreePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	require.NoError(t, ln.Close())
	return port
}

// collServer is a static or replay server running in a goroutine.
type collServer struct {
	port   uint16
	cancel context.CancelFunc
	done   chan error
}

// collStartServer runs serve on a free port and waits until it accepts connections.
func collStartServer(t *testing.T, serve func(ctx context.Context, port uint16) error) *collServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &collServer{port: collFreePort(t), cancel: cancel, done: make(chan error, 1)}
	go func() { s.done <- serve(ctx, s.port) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop after its context was canceled")
		}
	})

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case err := <-s.done:
			s.done <- err
			t.Fatalf("server exited before accepting connections: %v", err)
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			require.NoError(t, conn.Close())
			return s
		}
		require.True(t, time.Now().Before(deadline), "server never accepted connections: %v", err)
		time.Sleep(5 * time.Millisecond)
	}
}

// stop cancels the server and returns the error it exited with.
func (s *collServer) stop(t *testing.T) error {
	t.Helper()
	s.cancel()
	select {
	case err := <-s.done:
		s.done <- err
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after its context was canceled")
		return nil
	}
}

func (s *collServer) client(t *testing.T) *gomodbus.Client {
	t.Helper()
	c, err := gomodbus.New(gomodbus.Config{
		URL:     fmt.Sprintf("tcp://127.0.0.1:%d", s.port),
		Timeout: 2 * time.Second,
		Logger:  gomodbus.NopLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, c.Open())
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// collRequireException asserts that err is the given Modbus exception.
func collRequireException(t *testing.T, err error, want error) {
	t.Helper()
	require.Error(t, err)
	var ex *gomodbus.ExceptionError
	require.Truef(t, errors.As(err, &ex), "expected a Modbus exception, got %T: %v", err, err)
	require.ErrorIs(t, err, want)
}

func regsResp(fc byte, vals ...uint16) []byte {
	b := []byte{fc, byte(2 * len(vals))}
	for _, v := range vals {
		b = append(b, byte(v>>8), byte(v))
	}
	return b
}

func readReq(fc byte, start, qty uint16) []byte {
	return []byte{fc, byte(start >> 8), byte(start), byte(qty >> 8), byte(qty)}
}

// collSite is a capture of two devices: an inverter (target 1, unit 21) polled
// three times a second apart, with an identity read and an exception status,
// and a meter (target 2, unit 3) with input registers and coils.
func collSite() *capture.Capture {
	return &capture.Capture{
		Header: capture.Header{TickNS: 1000, WallStart: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		Targets: []capture.Target{
			{ID: 1, Label: "Inverter A", Transport: capture.TransportTCP, Host: "10.0.0.1", Port: 502, Unit: 21},
			{ID: 2, Label: "Meter", Transport: capture.TransportTCP, Host: "10.0.0.2", Port: 502, Unit: 3},
		},
		Streams: []capture.Stream{{ID: 1, Target: 1, Role: "poll"}, {ID: 2, Target: 1, Role: "identity"}, {ID: 3, Target: 2, Role: "poll"}},
		Samples: []capture.Sample{
			{Stream: 1, At: 0, Request: readReq(3, 10, 2), Response: regsResp(3, 1, 2)},
			{Stream: 3, At: 10, Request: readReq(4, 0, 2), Response: regsResp(4, 100, 200)},
			{Stream: 3, At: 20, Request: readReq(1, 0, 4), Response: []byte{0x01, 1, 0b0101}},
			{Stream: 1, At: 1_000_000, Request: readReq(3, 10, 2), Response: regsResp(3, 3, 4)},
			{Stream: 1, At: 1_000_100, Request: []byte{0x07}, Response: []byte{0x07, 0x6d}},
			{Stream: 1, At: 1_000_200, Request: []byte{0x0B}, Response: []byte{0x0B, 0x00, 0x00, 0x01, 0x08}},
			{Stream: 1, At: 1_000_300, Request: []byte{0x0C}, Response: []byte{0x0C, 0x08, 0x00, 0x00, 0x01, 0x08, 0x01, 0x21, 0x20, 0x00}},
			{Stream: 2, At: 1_500_000, Request: []byte{0x2B, 0x0E, 0x01, 0x00},
				Response: []byte{0x2B, 0x0E, 0x01, 0x01, 0x00, 0x00, 0x03, 0x00, 0x04, 'A', 'C', 'M', 'E', 0x01, 0x02, 'X', '1', 0x02, 0x03, '1', '.', '0'}},
			{Stream: 1, At: 2_000_000, Request: readReq(3, 10, 2), Response: regsResp(3, 5, 6)},
		},
		End: capture.End{Known: true, At: 3_000_000},
	}
}

func staticServer(t *testing.T, cfg config.StaticServerConfig) (*collServer, *collSyncBuffer) {
	t.Helper()
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		cfg.Port = port
		return ServeStatic(ctx, cfg, &progress)
	})
	return srv, &progress
}

func TestCollStaticServer_lastValuesOfEveryTarget(t *testing.T) {
	srv, progress := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite())})
	c := srv.client(t)
	ctx := context.Background()

	got, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{5, 6}, got, "the static server serves the last recorded values")
	got, err = c.ReadInputRegisters(ctx, 3, 0, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{100, 200}, got, "the second target answers on its own unit")
	bits, err := c.ReadCoils(ctx, 3, 1, 3)
	require.NoError(t, err)
	require.Equal(t, []bool{false, true, false}, bits)

	status, err := c.ReadExceptionStatus(ctx, 21)
	require.NoError(t, err)
	require.Equal(t, uint8(0x6d), status)
	counter, err := c.GetCommEventCounter(ctx, 21)
	require.NoError(t, err)
	require.Equal(t, gomodbus.CommEventCounterResponse{Status: 0, EventCount: 0x0108}, *counter)
	log, err := c.GetCommEventLog(ctx, 21)
	require.NoError(t, err)
	require.Equal(t, uint16(0x0121), log.MessageCount)
	require.Equal(t, []byte{0x20, 0x00}, log.Events)
	_, err = c.GetCommEventCounter(ctx, 3)
	collRequireException(t, err, gomodbus.ErrIllegalFunction) // never recorded for the meter
	err = c.WriteCoil(ctx, 3, 0, true)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)
	di, err := c.ReadDeviceIdentification(ctx, 21, gomodbus.ReadDeviceIDBasic, 0)
	require.NoError(t, err)
	require.Len(t, di.Objects, 3)
	require.Equal(t, "ACME", di.Objects[0].Value)
	require.Equal(t, "1.0", di.Objects[2].Value)

	text := progress.String()
	assert.Contains(t, text, "Static server ready on port")
	assert.Contains(t, text, "the last recorded values")
	assert.Contains(t, text, `Unit 3: target 2 "Meter", 2 samples`)
	assert.Contains(t, text, `Unit 21: target 1 "Inverter A", 7 samples`)
}

func TestCollStaticServer_answers(t *testing.T) {
	path := collWriteCapture(t, collSite())
	ctx := context.Background()

	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: path})
	c := srv.client(t)
	_, err := c.ReadHoldingRegisters(ctx, 21, 10, 3)
	collRequireException(t, err, gomodbus.ErrIllegalDataAddress) // 12 was never recorded
	_, err = c.ReadInputRegisters(ctx, 21, 0, 1)
	collRequireException(t, err, gomodbus.ErrIllegalFunction) // the inverter has no FC04 data
	_, err = c.ReadDeviceIdentification(ctx, 3, gomodbus.ReadDeviceIDBasic, 0)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)
	err = c.WriteRegister(ctx, 21, 10, 9)
	collRequireException(t, err, gomodbus.ErrIllegalFunction) // a replayed device is read-only
	_, err = c.ReadHoldingRegisters(ctx, 9, 10, 1)
	require.Error(t, err, "an unknown unit is not answered with data")

	srv, _ = staticServer(t, config.StaticServerConfig{InputFile: path, Missing: "zero"})
	got, err := srv.client(t).ReadHoldingRegisters(ctx, 21, 10, 3)
	require.NoError(t, err)
	require.Equal(t, []uint16{5, 6, 0}, got)

	srv, _ = staticServer(t, config.StaticServerConfig{InputFile: path, Missing: "silence"})
	c, err = gomodbus.New(gomodbus.Config{URL: fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), Timeout: 200 * time.Millisecond, Logger: gomodbus.NopLogger()})
	require.NoError(t, err)
	require.NoError(t, c.Open())
	t.Cleanup(func() { _ = c.Close() })
	_, err = c.ReadHoldingRegisters(ctx, 21, 10, 3)
	require.ErrorIs(t, err, gomodbus.ErrRequestTimedOut)
}

func TestCollStaticServer_atAndTargetSelection(t *testing.T) {
	path := collWriteCapture(t, collSite())
	ctx := context.Background()
	srv, progress := staticServer(t, config.StaticServerConfig{InputFile: path, Target: 1, Unit: 1, At: "1500ms"})
	c := srv.client(t)
	got, err := c.ReadHoldingRegisters(ctx, 1, 10, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{3, 4}, got, "the values at 1.5 s")
	_, err = c.ReadInputRegisters(ctx, 3, 0, 1)
	require.Error(t, err, "the meter is not served")
	assert.Contains(t, progress.String(), "the values at 1.5s")
}

func TestCollServer_errors(t *testing.T) {
	ctx := context.Background()
	site := collWriteCapture(t, collSite())

	err := ServeStatic(ctx, config.StaticServerConfig{Port: 1, InputFile: filepath.Join(t.TempDir(), "missing.mbcap")}, nil)
	coreRequireKind(t, err, errs.KindOutput, errs.CodeMCAPLoadFailed)

	v1 := filepath.Join(t.TempDir(), "old.mcap")
	require.NoError(t, os.WriteFile(v1, []byte("MCAP\x01\x00\x02{}"), 0o600))
	err = ServeReplay(ctx, config.ReplayServerConfig{Port: 1, InputFile: v1}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeCaptureLegacyFormat)
	require.ErrorIs(t, err, capfile.ErrLegacyMCAP)

	err = ServeStatic(ctx, config.StaticServerConfig{Port: 1, InputFile: site, Target: 9}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	assert.Contains(t, err.Error(), `1 "Inverter A" (unit 21), 2 "Meter" (unit 3)`)

	err = ServeStatic(ctx, config.StaticServerConfig{Port: 1, InputFile: site, Unit: 5}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidFlagCombination)

	same := collSite()
	same.Targets[1].Unit = 21
	err = ServeStatic(ctx, config.StaticServerConfig{Port: 1, InputFile: collWriteCapture(t, same)}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidFlagCombination)
	assert.Contains(t, err.Error(), "both answer on unit 21")

	for _, cfg := range []config.ReplayServerConfig{
		{Port: 1, InputFile: site, Speed: "fast"},
		{Port: 1, InputFile: site, Missing: "panic"},
		{Port: 1, InputFile: site, BeforeFirst: "later"},
		{Port: 1, InputFile: site, From: "-1s"},
	} {
		err := ServeReplay(ctx, cfg, nil)
		coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, ServeStatic(canceled, config.StaticServerConfig{Port: 1, InputFile: site}, nil), context.Canceled)
}

func TestCollReplayServer_followsTheRecording(t *testing.T) {
	path := collWriteCapture(t, collSite())
	var progress collSyncBuffer
	// 3 s of capture at 10x: 300 ms per loop, played twice.
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return ServeReplay(ctx, config.ReplayServerConfig{Port: port, InputFile: path, Target: 1, Speed: "10", Loops: 2}, &progress)
	})
	c := srv.client(t)
	ctx := context.Background()

	seen := map[uint16]bool{}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(seen) < 3 {
		got, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
		require.NoError(t, err)
		seen[got[0]] = true
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, map[uint16]bool{1: true, 3: true, 5: true}, seen, "every recorded value is served in turn")

	// After two loops (600 ms) the last values stay.
	time.Sleep(700 * time.Millisecond)
	for range 5 {
		got, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
		require.NoError(t, err)
		require.Equal(t, []uint16{5, 6}, got)
	}
	assert.Contains(t, progress.String(), "at 10x, looping 2 time(s)")
	require.ErrorIs(t, srv.stop(t), context.Canceled)
}

func TestCollReplayServer_beforeFirst(t *testing.T) {
	path := collWriteCapture(t, collSite())
	ctx := context.Background()
	for _, tc := range []struct {
		before string
		ok     bool
	}{{"hold", true}, {"absent", false}} {
		srv := collStartServer(t, func(ctx context.Context, port uint16) error {
			// Speed 0.001: the replay stays near the start, before the identity read at 1.5 s.
			return ServeReplay(ctx, config.ReplayServerConfig{Port: port, InputFile: path, Speed: "0.001", BeforeFirst: tc.before}, nil)
		})
		_, err := srv.client(t).ReadDeviceIdentification(ctx, 21, gomodbus.ReadDeviceIDBasic, 0)
		if tc.ok {
			require.NoError(t, err, tc.before)
		} else {
			collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
		}
	}
}

func TestCollServer_truncatedCapture(t *testing.T) {
	path := collWriteCapture(t, collSite())
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	cut := filepath.Join(t.TempDir(), "cut.mbcap")
	require.NoError(t, os.WriteFile(cut, b[:len(b)-30], 0o600))
	srv, progress := staticServer(t, config.StaticServerConfig{InputFile: cut})
	got, err := srv.client(t).ReadHoldingRegisters(context.Background(), 21, 10, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{5, 6}, got)
	assert.True(t, strings.Contains(progress.String(), "truncated"), progress.String())
}

// With --debug the server names every request the capture holds nothing for,
// and on shutdown it reports what it served (go-modbus ServerMetrics).
func TestCollServer_debugAndStats(t *testing.T) {
	srv, progress := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite()), Debug: true})
	c := srv.client(t)
	ctx := context.Background()
	_, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
	require.NoError(t, err)
	_, err = c.ReadHoldingRegisters(ctx, 21, 10, 3)
	require.Error(t, err)
	_, err = c.GetCommEventCounter(ctx, 3)
	require.Error(t, err)
	require.ErrorIs(t, srv.stop(t), context.Canceled)
	text := progress.String()
	assert.Contains(t, text, "DEBUG [serve] unit 21 FC03 10 x3: not in the capture")
	assert.Contains(t, text, "DEBUG [serve] unit 3 FC11: not in the capture")
	assert.Contains(t, text, "📊 Served 3 requests: 1 answered, 2 with an exception, 0 left unanswered; 2 asked for data the capture does not hold")
}

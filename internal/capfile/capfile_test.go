package capfile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exchange(req, resp []byte) modbus.PDUExchange {
	return modbus.PDUExchange{UnitID: 1, Request: req, Response: resp, Received: time.Now()}
}

func newRecorder(t *testing.T, opts RecorderOptions) (*Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rec.mbcap")
	r, err := Create(path, opts)
	require.NoError(t, err)
	require.NoError(t, r.AddTarget(capture.Target{ID: 1, Unit: 1}))
	require.NoError(t, r.AddStream(capture.Stream{ID: 1, Target: 1, Role: capture.RolePoll}))
	// Definitions are written once.
	require.NoError(t, r.AddTarget(capture.Target{ID: 1, Unit: 1}))
	require.NoError(t, r.AddStream(capture.Stream{ID: 1, Target: 1}))
	return r, path
}

// attempt plays one request attempt through the recorder: the tap sees the
// exchanges, then the attempt ends with err.
func attempt(t *testing.T, r *Recorder, err error, exs ...modbus.PDUExchange) {
	t.Helper()
	ctx, tok := r.Begin(context.Background())
	for _, ex := range exs {
		r.Tap().OnPDU(ctx, ex)
	}
	require.NoError(t, r.Commit(tok, 1, err))
}

func TestRecorder_commitsSuccessfulAttemptsOnly(t *testing.T) {
	r, path := newRecorder(t, RecorderOptions{})
	ok := exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x03, 2, 0, 7})
	attempt(t, r, nil, ok)
	// A busy device answers an exception; the retry succeeds. Only the retry is kept.
	attempt(t, r, &modbus.ExceptionError{}, exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x83, 6}))
	attempt(t, r, nil, exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x03, 2, 0, 8}))
	// A response the method rejects (wrong byte count) is not kept either.
	attempt(t, r, errors.New("protocol error"), exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x03, 4, 0, 8, 0, 9}))
	// A paged FC43 read keeps every page of the call.
	attempt(t, r, nil,
		exchange([]byte{0x2B, 0x0E, 0x01, 0x00}, []byte{0x2B, 0x0E, 0x01, 0x01, 0xFF, 0x01, 0x01, 0x00, 0x01, 'A'}),
		exchange([]byte{0x2B, 0x0E, 0x01, 0x01}, []byte{0x2B, 0x0E, 0x01, 0x01, 0x00, 0x00, 0x01, 0x01, 0x01, 'B'}))
	// A write is never recorded, and an exchange outside an attempt is ignored.
	attempt(t, r, nil, exchange([]byte{0x06, 0, 1, 0, 2}, []byte{0x06, 0, 1, 0, 2}))
	r.Tap().OnPDU(context.Background(), ok)
	require.Equal(t, 4, r.Samples())
	require.NoError(t, r.Close())
	require.NoError(t, r.Close(), "Close is idempotent")
	attempt(t, r, nil, ok) // after Close: dropped

	l, err := Load(path)
	require.NoError(t, err)
	require.False(t, l.Truncated)
	require.True(t, l.Capture.End.Known)
	require.Len(t, l.Samples, 4)
	assert.Equal(t, []byte{0x03, 2, 0, 8}, l.Samples[1].Response)
	assert.Equal(t, byte(0x2B), l.Samples[3].Request[0])
	assert.Equal(t, Producer, l.Header.Producer)
	for i := 1; i < len(l.Samples); i++ {
		assert.GreaterOrEqual(t, l.Samples[i].At, l.Samples[i-1].At)
	}
}

func TestRecorder_exceptions(t *testing.T) {
	r, path := newRecorder(t, RecorderOptions{Exceptions: true, Tags: map[string]string{"site": "lab"}})
	attempt(t, r, &modbus.ExceptionError{}, exchange([]byte{0x03, 0x27, 0x10, 0, 1}, []byte{0x83, 2}))
	attempt(t, r, errors.New("timeout"), exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x03, 2, 0, 1}))
	require.NoError(t, r.Close())
	l, err := Load(path)
	require.NoError(t, err)
	require.Len(t, l.Samples, 1)
	assert.True(t, l.Samples[0].IsException())
	assert.True(t, l.Header.Exceptions)
	assert.Equal(t, "lab", l.Header.Tags["site"])
}

func TestTargetFor(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want capture.Target
	}{
		{"tcp://10.0.0.1:502", capture.Target{Transport: capture.TransportTCP, Host: "10.0.0.1", Port: 502}},
		{"tcp+tls://plc:802", capture.Target{Transport: capture.TransportTLS, Host: "plc", Port: 802}},
		{"udp://10.0.0.2:502", capture.Target{Transport: capture.TransportUDP, Host: "10.0.0.2", Port: 502}},
		{"rtuovertcp://gw:4001", capture.Target{Transport: capture.TransportRTUOverTCP, Host: "gw", Port: 4001}},
		{"asciiovertcp://gw:4002", capture.Target{Transport: capture.TransportUnknown, Host: "gw", Port: 4002}},
		{"rtu:///dev/ttyUSB0", capture.Target{Transport: capture.TransportRTU,
			Serial: capture.Serial{Device: "/dev/ttyUSB0", Baud: 19200, Parity: capture.ParityEven, DataBits: 8, StopBits: 1}}},
	} {
		got := TargetFor(3, tc.url, 7, config.SerialConfig{Baud: 19200, Parity: "even", DataBits: 8, StopBits: 1}, "x")
		assert.Equal(t, tc.want.Transport, got.Transport, tc.url)
		assert.Equal(t, tc.want.Host, got.Host, tc.url)
		assert.Equal(t, tc.want.Port, got.Port, tc.url)
		assert.Equal(t, tc.want.Serial, got.Serial, tc.url)
		assert.Equal(t, uint32(3), got.ID)
		assert.Equal(t, uint8(7), got.Unit)
		assert.Equal(t, tc.url, got.Tags["url"])
	}
}

func TestOpenReader(t *testing.T) {
	var jl bytes.Buffer
	h := capture.Header{TickNS: 1000, WallStart: time.Unix(0, 0).UTC()}
	w, err := jsonl.NewWriter(&jl, h)
	require.NoError(t, err)
	require.NoError(t, w.Close(capture.End{}))
	for _, tc := range []struct {
		name string
		in   []byte
		enc  Encoding
		err  error
	}{
		{"jsonl", jl.Bytes(), JSONL, nil},
		{"csv", []byte("t,version,tick_ns,wall_start\nheader,0.1,1000,1970-01-01T00:00:00Z\n"), CSV, nil},
		{"v1", []byte("MCAP\x01\x00\x02{}"), "", ErrLegacyMCAP},
		{"other", []byte("hello"), "", capture.ErrCorrupt},
	} {
		_, enc, err := OpenReader(bytes.NewReader(tc.in))
		if tc.err != nil {
			require.ErrorIs(t, err, tc.err, tc.name)
			continue
		}
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.enc, enc, tc.name)
	}
	assert.True(t, strings.Contains(ErrLegacyMCAP.Error(), "modbusctl 3"))
}

func TestLoad_truncatedAndErrors(t *testing.T) {
	r, path := newRecorder(t, RecorderOptions{})
	attempt(t, r, nil, exchange([]byte{0x03, 0, 0, 0, 1}, []byte{0x03, 2, 0, 7}))
	require.NoError(t, r.Close())
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	cut := filepath.Join(t.TempDir(), "cut.mbcap")
	require.NoError(t, os.WriteFile(cut, b[:len(b)-20], 0o600))
	l, err := Load(cut)
	require.NoError(t, err)
	assert.True(t, l.Truncated)
	assert.Len(t, l.Samples, 1)
	assert.Equal(t, l.Samples[0].At, l.End(), "a truncated capture holds until its last sample")
	assert.Equal(t, `1 (unit 1)`, l.ListTargets())

	_, err = Load(filepath.Join(t.TempDir(), "missing.mbcap"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWrapError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		kind errs.Kind
		code string
	}{
		{fmt.Errorf("x.mcap: %w", ErrLegacyMCAP), errs.KindInvalidInput, errs.CodeCaptureLegacyFormat},
		{fmt.Errorf("%w; choose one: ...", ErrAmbiguousSelection), errs.KindInvalidInput, errs.CodeCaptureSelectionAmbiguous},
		{fmt.Errorf("%w for target 2", ErrEmptySelection), errs.KindInvalidInput, errs.CodeCaptureSelectionEmpty},
		{fmt.Errorf("x: %w", capture.ErrTruncated), errs.KindOutput, errs.CodeMCAPLoadFailed},
		{os.ErrNotExist, errs.KindOutput, errs.CodeMCAPLoadFailed},
		{errs.InvalidInput(errs.CodeInvalidInput, "already typed", nil), errs.KindInvalidInput, errs.CodeInvalidInput},
	} {
		e, ok := errs.As(WrapError(tc.err))
		require.True(t, ok, "%v", tc.err)
		assert.Equal(t, tc.kind, e.Kind, "%v", tc.err)
		assert.Equal(t, tc.code, e.Code, "%v", tc.err)
		assert.ErrorIs(t, WrapError(tc.err), tc.err)
	}
	assert.NoError(t, WrapError(nil))
}

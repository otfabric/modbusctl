package modbus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreCollectRead_everyFunctionCode(t *testing.T) {
	dev := mbtest.NewDevice().
		SetCoils(0, true, false, true, true, false, false, false, false, true).
		SetDiscreteInputs(10, false, true, true).
		SetHolding(100, 0x1234, 0xABCD, 0x0001).
		SetInput(200, 0xFFFF, 0x0000)
	dc := coreTCP(t, dev)

	tests := []struct {
		name    string
		fc      uint8
		start   uint16
		count   uint16
		wantRaw []byte
	}{
		{name: "coils", fc: 1, start: 0, count: 9, wantRaw: []byte{0x0D, 0x01}},
		{name: "discrete inputs", fc: 2, start: 10, count: 3, wantRaw: []byte{0x06}},
		{name: "holding registers", fc: 3, start: 100, count: 3, wantRaw: []byte{0x12, 0x34, 0xAB, 0xCD, 0x00, 0x01}},
		{name: "input registers", fc: 4, start: 200, count: 2, wantRaw: []byte{0xFF, 0xFF, 0x00, 0x00}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := coreMcapPath(t)
			before := time.Now().UnixNano()
			res, err := CollectRead(context.Background(), config.ReadConfig{
				DeviceConfig: dc, Timeout: coreTimeoutMS, Function: tc.fc,
				StartAddress: tc.start, RegisterCount: tc.count, OutputFile: out,
			}, nil)
			require.NoError(t, err)

			assert.Equal(t, dc.URL, res.Target)
			assert.Equal(t, uint8(1), res.UnitID)
			assert.Equal(t, tc.fc, res.Function)
			assert.Equal(t, tc.start, res.StartAddress)
			assert.Equal(t, tc.count, res.RegisterCount)
			assert.Equal(t, len(tc.wantRaw), res.RawByteCount)
			assert.Equal(t, fmt.Sprintf("% X", tc.wantRaw), res.RawDataHex)
			assert.Equal(t, out, res.McapOutputPath)

			records, header := coreLoadMcap(t, out)
			host, portStr, err := net.SplitHostPort(strings.TrimPrefix(dc.URL, "tcp://"))
			require.NoError(t, err)
			port, err := strconv.Atoi(portStr)
			require.NoError(t, err)
			assert.Equal(t, host, header.IP)
			assert.Equal(t, uint16(port), header.Port)
			assert.Equal(t, uint8(1), header.Unit)
			assert.Equal(t, tc.fc, header.Function)
			assert.GreaterOrEqual(t, header.StartTime, before)

			require.Len(t, records, 1)
			rec := records[0]
			assert.Equal(t, uint32(0), rec.Iteration)
			assert.Equal(t, tc.start, rec.StartAddress)
			assert.Equal(t, tc.count, rec.RegisterCount)
			assert.Equal(t, tc.wantRaw, rec.Data)
			assert.GreaterOrEqual(t, rec.RequestTimestamp, before)
			assert.GreaterOrEqual(t, rec.ResponseTimestamp, rec.RequestTimestamp)
		})
	}
	assert.Equal(t, 4, dev.RequestCount(0))
}

func TestCoreCollectRead_ipAndPortInsteadOfURL(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(5, 42)
	host, portStr, err := net.SplitHostPort(dev.ServeTCP(t))
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	res, err := CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: config.DeviceConfig{IP: host, Port: uint16(port), Unit: 9},
		Timeout:      coreTimeoutMS, Function: 3, StartAddress: 5, RegisterCount: 1,
		OutputFile: coreMcapPath(t),
	}, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "tcp://"+net.JoinHostPort(host, portStr), res.Target)
	assert.Equal(t, "00 2A", res.RawDataHex)
	require.Len(t, dev.Requests(), 1)
	assert.Equal(t, uint8(9), dev.Requests()[0].Unit)
	assert.Equal(t, []byte{0x03, 0x00, 0x05, 0x00, 0x01}, dev.Requests()[0].PDU)
}

// An output path ending in a slash is a directory: the capture gets an auto-generated name.
func TestCoreCollectRead_outputDirectory(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 7)
	dir := t.TempDir()
	res, err := CollectRead(context.Background(), config.ReadConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
		OutputFile: dir + "/",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, dir, filepath.Dir(res.McapOutputPath))
	assert.Regexp(t, `^modbusctl_read_\d{8}_\d{6}\.mcap$`, filepath.Base(res.McapOutputPath))
	records, _ := coreLoadMcap(t, res.McapOutputPath)
	require.Len(t, records, 1)
	assert.Equal(t, []byte{0x00, 0x07}, records[0].Data)
}

func TestCoreReadCaptureMcapPath(t *testing.T) {
	assert.Equal(t, "/x/y.mcap", readCaptureMcapPath(config.ReadConfig{OutputFile: "/x/y.mcap"}))
	assert.Regexp(t, `^/x/modbusctl_read_\d{8}_\d{6}\.mcap$`, readCaptureMcapPath(config.ReadConfig{OutputFile: "/x/"}))
	// No output file: an auto-named capture in the current directory.
	assert.Regexp(t, `^modbusctl_read_\d{8}_\d{6}\.mcap$`, readCaptureMcapPath(config.ReadConfig{}))
}

func TestCoreAutoCaptureMcapPath_defaultKind(t *testing.T) {
	assert.Regexp(t, `^d/modbusctl_capture_\d{8}_\d{6}\.mcap$`, AutoCaptureMcapPath("d", ""))
}

func TestCoreCollectRead_failures(t *testing.T) {
	t.Run("connect refused", func(t *testing.T) {
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t), Unit: 1},
			Timeout:      coreShortTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: coreMcapPath(t),
		}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
		coreRequireKind(t, err, errs.KindTransport, errs.CodeTransportConnectFailed)
	})

	t.Run("invalid url", func(t *testing.T) {
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: config.DeviceConfig{URL: "bogus://127.0.0.1:502", Unit: 1},
			Timeout:      coreShortTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: coreMcapPath(t),
		}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
	})

	t.Run("output file cannot be created", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1,
			OutputFile: filepath.Join(t.TempDir(), "missing", "out.mcap"),
		}, nil)
		coreRequireKind(t, err, errs.KindOutput, errs.CodeOutputFileCreateFailed)
		assert.Zero(t, dev.RequestCount(0), "no request may be sent when the capture cannot be written")
	})

	t.Run("modbus exception", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 50, RegisterCount: 2,
			OutputFile: coreMcapPath(t),
		}, nil)
		var ex *gomodbus.ExceptionError
		require.ErrorAs(t, err, &ex)
		assert.Equal(t, uint8(mbtest.ExIllegalDataAddress), uint8(ex.ExceptionCode))
		coreRequireKind(t, err, errs.KindModbus, errs.CodeModbusException)
		assert.Equal(t, 1, dev.RequestCount(3), "a read is not retried on an exception")
	})

	for _, fc := range []uint8{1, 2, 4} {
		t.Run(fmt.Sprintf("modbus exception, function code %d", fc), func(t *testing.T) {
			dev := mbtest.NewDevice()
			_, err := CollectRead(context.Background(), config.ReadConfig{
				DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: fc, StartAddress: 5, RegisterCount: 3,
				OutputFile: coreMcapPath(t),
			}, nil)
			require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
			assert.Equal(t, 1, dev.RequestCount(fc))
		})
	}

	t.Run("timeout", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
		start := time.Now()
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, RegisterCount: 1,
			OutputFile: coreMcapPath(t),
		}, nil)
		coreRequireKind(t, err, errs.KindTimeout, errs.CodeRequestTimeout)
		assert.GreaterOrEqual(t, time.Since(start), coreShortTimeoutMS*time.Millisecond)
		assert.Equal(t, 1, dev.RequestCount(0))
	})

	t.Run("unsupported function code", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 1)
		_, err := CollectRead(context.Background(), config.ReadConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 7, RegisterCount: 1,
			OutputFile: coreMcapPath(t),
		}, nil)
		coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		assert.Contains(t, err.Error(), "unsupported function code: 7")
		assert.Zero(t, dev.RequestCount(0))
	})
}

// coreOpenLink opens a device link to dev with the given debug setting.
func coreOpenLink(t *testing.T, url string, debug bool) *deviceLink {
	t.Helper()
	link, err := openDeviceLink(url, coreTimeoutMS, debug, config.SerialConfig{})
	require.NoError(t, err)
	t.Cleanup(link.Close)
	return link
}

func TestCoreReadRegisters_retriesBusyAndGatewayExceptions(t *testing.T) {
	for _, code := range []uint8{mbtest.ExServerDeviceBusy, mbtest.ExGatewayTargetFailed} {
		t.Run(fmt.Sprintf("exception 0x%02X", code), func(t *testing.T) {
			dev := mbtest.NewDevice().SetHolding(10, 0xBEEF)
			dev.OnRequest = coreFirstN(2, mbtest.Action{Exception: code})
			link := coreOpenLink(t, coreTCP(t, dev).URL, true)

			var progress bytes.Buffer
			data, reqTS, resTS, err := readRegisters(context.Background(), link, 3, 10, 1, 5, 1, 0, &progress)
			require.NoError(t, err)
			assert.Equal(t, []byte{0xBE, 0xEF}, data)
			assert.Positive(t, reqTS)
			assert.GreaterOrEqual(t, resTS, reqTS)
			assert.Equal(t, 3, dev.RequestCount(3))
			assert.Equal(t, 2, strings.Count(progress.String(), "Retrying due to Modbus read exception on address 10 with count 1"))
			assert.Contains(t, progress.String(), "(attempt 2)")
		})
	}
}

func TestCoreReadRegisters_busyUntilAttemptsExhausted(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 1)
	dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Exception: mbtest.ExServerDeviceBusy} }
	link := coreOpenLink(t, coreTCP(t, dev).URL, false)

	var progress bytes.Buffer
	data, _, _, err := readRegisters(context.Background(), link, 3, 10, 1, 3, 1, 0, &progress)
	require.ErrorIs(t, err, gomodbus.ErrServerDeviceBusy)
	assert.Nil(t, data)
	assert.Equal(t, 3, dev.RequestCount(3))
	assert.Empty(t, progress.String(), "retry chatter is debug-only")
}

// A non-retryable exception is returned at once, whatever the number of attempts.
func TestCoreReadRegisters_illegalAddressNotRetried(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 1)
	link := coreOpenLink(t, coreTCP(t, dev).URL, true)

	_, reqTS, resTS, err := readRegisters(context.Background(), link, 3, 500, 1, 5, 1, 0, nil)
	require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
	assert.Equal(t, 1, dev.RequestCount(3))
	assert.Positive(t, reqTS)
	assert.GreaterOrEqual(t, resTS, reqTS)
}

func TestCoreReadRegisters_delayBeforeEveryAttempt(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 1)
	dev.OnRequest = coreFirstN(1, mbtest.Action{Exception: mbtest.ExServerDeviceBusy})
	link := coreOpenLink(t, coreTCP(t, dev).URL, true)

	var progress bytes.Buffer
	start := time.Now()
	_, _, _, err := readRegisters(context.Background(), link, 3, 10, 1, 2, 1, 30, &progress)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
	assert.Equal(t, 2, strings.Count(progress.String(), "Waiting 30 ms before retrying"))
}

func TestCoreReadRegisters_canceledWhileWaiting(t *testing.T) {
	t.Run("during the configured delay", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(10, 1)
		link := coreOpenLink(t, coreTCP(t, dev).URL, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, _, err := readRegisters(ctx, link, 3, 10, 1, 2, 1, 1000, nil)
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, dev.RequestCount(0))
	})

	t.Run("during the pause after a busy exception", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(10, 1)
		ctx, cancel := context.WithCancel(context.Background())
		dev.OnRequest = func(mbtest.Request) mbtest.Action {
			cancel()
			return mbtest.Action{Exception: mbtest.ExServerDeviceBusy}
		}
		link := coreOpenLink(t, coreTCP(t, dev).URL, false)
		_, _, _, err := readRegisters(ctx, link, 3, 10, 1, 5, 1, 0, nil)
		require.Error(t, err)
		assert.Equal(t, 1, dev.RequestCount(0), "no further attempt after cancellation")
	})
}

// A connection closed by the device is re-dialed and the read repeated.
func TestCoreReadRegisters_reconnectsAfterClosedConnection(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 0x0102, 0x0304)
	dev.OnRequest = coreFirstN(1, mbtest.Action{Close: true})
	link := coreOpenLink(t, coreTCP(t, dev).URL, true)

	var progress bytes.Buffer
	data, _, _, err := readRegisters(context.Background(), link, 3, 10, 2, 2, 1, 0, &progress)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3, 4}, data)
	assert.Equal(t, 2, dev.RequestCount(3))
	assert.Contains(t, progress.String(), "Reconnecting due to connection error (attempt 1)")
}

// With a single attempt the connection error itself is reported, but the link is re-dialed
// so that the next read works.
func TestCoreReadRegisters_singleAttemptReportsConnectionError(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(10, 5)
	dev.OnRequest = coreFirstN(1, mbtest.Action{Close: true})
	link := coreOpenLink(t, coreTCP(t, dev).URL, false)

	_, _, _, err := readRegisters(context.Background(), link, 3, 10, 1, 1, 1, 0, nil)
	require.Error(t, err)
	assert.True(t, shouldReconnect(err), "unexpected error: %v", err)
	outcome, code := classifyOutcome(err, 0, 0)
	assert.Equal(t, ScanOutcomeTransport, outcome)
	assert.Zero(t, code)

	data, _, _, err := readRegisters(context.Background(), link, 3, 10, 1, 1, 1, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 5}, data)
}

// When the device is gone for good the re-dial failure is returned as a connection error,
// and the closed link keeps failing the same way on later reads.
func TestCoreReadRegisters_reconnectFails(t *testing.T) {
	link := coreOpenLink(t, coreDropListener(t), false)

	_, _, _, err := readRegisters(context.Background(), link, 3, 0, 1, 3, 1, 0, nil)
	require.ErrorIs(t, err, ErrTCPConnection)
	assert.Nil(t, link.client, "a failed re-dial leaves the link closed")

	_, _, _, err = readRegisters(context.Background(), link, 3, 0, 1, 3, 1, 0, nil)
	require.ErrorIs(t, err, ErrTCPConnection)
}

// A link closed earlier is re-dialed transparently by the next read.
func TestCoreReadRegisters_redialsClosedLink(t *testing.T) {
	dev := mbtest.NewDevice().SetInput(3, 0x00FF)
	link := coreOpenLink(t, coreTCP(t, dev).URL, false)
	link.Close()
	link.Close() // Safe to call twice.
	require.Nil(t, link.client)

	data, _, _, err := readRegisters(context.Background(), link, 4, 3, 1, 1, 1, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0xFF}, data)
	assert.NotNil(t, link.client)
}

func TestCoreExecuteReadTask(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 0xAAAA, 0xBBBB)
	dc := coreTCP(t, dev)
	link := coreOpenLink(t, dc.URL, false)
	cfg := config.ScanConfig{DeviceConfig: dc, Function: 3}

	ok := executeReadTask(context.Background(), link, cfg, ScanTask{Start: 0, Count: 2}, nil)
	assert.True(t, ok.Success)
	assert.Equal(t, uint16(0), ok.Start)
	assert.Equal(t, uint16(2), ok.Count)
	assert.Equal(t, []byte{0xAA, 0xAA, 0xBB, 0xBB}, ok.Data)
	assert.Equal(t, ScanOutcomeSuccess, ok.OutcomeType)
	assert.Zero(t, ok.ExceptionCode)
	assert.NoError(t, ok.Err)
	assert.Equal(t, ok.ResponseTimestamp-ok.RequestTimestamp, ok.RTTNanos)

	bad := executeReadTask(context.Background(), link, cfg, ScanTask{Start: 1, Count: 2}, nil)
	assert.False(t, bad.Success)
	assert.Nil(t, bad.Data)
	assert.Equal(t, ScanOutcomeException, bad.OutcomeType)
	assert.Equal(t, uint8(mbtest.ExIllegalDataAddress), bad.ExceptionCode)
	assert.Error(t, bad.Err)
}

func TestCoreClassifyOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ScanOutcomeType
		code uint8
	}{
		{name: "nil", err: nil, want: ScanOutcomeSuccess},
		{name: "exception", err: &gomodbus.ExceptionError{ExceptionCode: 0x02}, want: ScanOutcomeException, code: 0x02},
		{name: "wrapped exception", err: fmt.Errorf("read: %w", &gomodbus.ExceptionError{ExceptionCode: 0x06}), want: ScanOutcomeException, code: 0x06},
		{name: "request timeout", err: fmt.Errorf("x: %w", gomodbus.ErrRequestTimedOut), want: ScanOutcomeTimeout},
		{name: "deadline", err: context.DeadlineExceeded, want: ScanOutcomeTimeout},
		{name: "protocol error", err: gomodbus.ErrProtocolError, want: ScanOutcomeProtocol},
		{name: "bad unit id", err: gomodbus.ErrBadUnitID, want: ScanOutcomeProtocol},
		{name: "bad transaction id", err: gomodbus.ErrBadTransactionID, want: ScanOutcomeProtocol},
		{name: "unknown protocol id", err: gomodbus.ErrUnknownProtocolID, want: ScanOutcomeProtocol},
		{name: "eof", err: io.EOF, want: ScanOutcomeTransport},
		{name: "connection reset", err: &net.OpError{Op: "read", Err: syscall.ECONNRESET}, want: ScanOutcomeTransport},
		{name: "canceled", err: context.Canceled, want: ScanOutcomeUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, code := classifyOutcome(tc.err, 1, 2)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.code, code)
		})
	}
}

func TestCoreShouldRetryAndReconnect(t *testing.T) {
	assert.False(t, shouldRetry(nil))
	assert.True(t, shouldRetry(gomodbus.ErrServerDeviceBusy))
	assert.True(t, shouldRetry(fmt.Errorf("x: %w", gomodbus.ErrGWTargetFailedToRespond)))
	assert.False(t, shouldRetry(gomodbus.ErrIllegalDataAddress))
	assert.False(t, shouldRetry(io.EOF))

	reconnect := []error{
		syscall.EPIPE,
		syscall.ENETRESET,
		fmt.Errorf("write: %w", syscall.EPIPE),
		errors.New("write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe"),
		errors.New("unexpected EOF"),
		errors.New("read: connection reset by peer"),
		&net.OpError{Op: "write", Err: errors.New("broken pipe")},
	}
	for _, err := range reconnect {
		assert.True(t, shouldReconnect(err), "%v", err)
	}
	keep := []error{
		syscall.EINVAL,
		&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
		gomodbus.ErrRequestTimedOut,
		errors.New("something else"),
	}
	for _, err := range keep {
		assert.False(t, shouldReconnect(err), "%v", err)
	}
}

func TestCoreClientConfig(t *testing.T) {
	assert.Equal(t, 10*time.Second, clientRequestTimeout(0))
	assert.Equal(t, 250*time.Millisecond, clientRequestTimeout(250))

	assert.Equal(t, defaultDialTimeout, dialTimeoutForRequest(0))
	assert.Equal(t, defaultDialTimeout, dialTimeoutForRequest(-time.Second))
	assert.Equal(t, 200*time.Millisecond, dialTimeoutForRequest(50*time.Millisecond), "floor")
	assert.Equal(t, time.Second, dialTimeoutForRequest(time.Second))
	assert.Equal(t, defaultDialTimeout, dialTimeoutForRequest(time.Minute), "cap")

	quiet := buildClientConfig("tcp://h:1", time.Second, false)
	assert.Equal(t, "tcp://h:1", quiet.URL)
	assert.Equal(t, time.Second, quiet.Timeout)
	assert.Equal(t, defaultDialTimeout, quiet.DialTimeout)
	assert.NotNil(t, quiet.Logger)
	debug := buildClientConfig("tcp://h:1", time.Second, true)
	assert.NotNil(t, debug.Logger)
	assert.NotEqual(t, fmt.Sprintf("%T", quiet.Logger), fmt.Sprintf("%T", debug.Logger), "debug selects a real logger")

	disc := discoveryClientConfig("tcp://h:1", false)
	assert.Equal(t, discoveryModbusTimeout, disc.Timeout)
	assert.Equal(t, discoveryDialTimeout, disc.DialTimeout)

	dev := buildDeviceClientConfig("rtu:///dev/null", 300, false, config.SerialConfig{Baud: 9600, DataBits: 7, StopBits: 2, Parity: "odd"})
	assert.Equal(t, 300*time.Millisecond, dev.Timeout)
	assert.Equal(t, 300*time.Millisecond, dev.DialTimeout)
	assert.Equal(t, uint(9600), dev.Speed)
	assert.Equal(t, uint(7), dev.DataBits)
	assert.Equal(t, uint(2), dev.StopBits)
	assert.Equal(t, uint(gomodbus.ParityOdd), uint(dev.Parity))
}

func TestCoreApplySerialConfig_parity(t *testing.T) {
	tests := []struct {
		parity string
		want   uint
	}{
		{parity: "", want: uint(gomodbus.ParityNone)},
		{parity: "none", want: uint(gomodbus.ParityNone)},
		{parity: "even", want: uint(gomodbus.ParityEven)},
		{parity: "EVEN", want: uint(gomodbus.ParityEven)},
		{parity: "odd", want: uint(gomodbus.ParityOdd)},
		{parity: "mark", want: uint(gomodbus.ParityNone)},
	}
	for _, tc := range tests {
		t.Run("parity "+tc.parity, func(t *testing.T) {
			for _, url := range []string{"rtu:///dev/ttyUSB0", "ascii:///dev/ttyUSB0"} {
				conf := gomodbus.Config{URL: url}
				applySerialConfig(&conf, config.SerialConfig{Baud: 4800, DataBits: 7, StopBits: 1, Parity: tc.parity})
				assert.Equal(t, tc.want, uint(conf.Parity), url)
				assert.Equal(t, uint(4800), conf.Speed, url)
				assert.Equal(t, uint(7), conf.DataBits, url)
				assert.Equal(t, uint(1), conf.StopBits, url)
			}
		})
	}
}

// Serial-line settings mean nothing on a network URL and are left out of its configuration.
func TestCoreApplySerialConfig_ignoredForNetworkURLs(t *testing.T) {
	for _, url := range []string{"tcp://127.0.0.1:502", "udp://127.0.0.1:502", "rtuovertcp://127.0.0.1:502", "asciiovertcp://127.0.0.1:502"} {
		conf := gomodbus.Config{URL: url}
		applySerialConfig(&conf, config.SerialConfig{Baud: 4800, DataBits: 7, StopBits: 2, Parity: "even"})
		assert.Equal(t, gomodbus.Config{URL: url}, conf, url)
	}
}

func TestCoreValidateAndConnect(t *testing.T) {
	t.Run("invalid configuration", func(t *testing.T) {
		client, cleanup, err := validateAndConnect(gomodbus.Config{URL: "bogus://x"})
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Nil(t, cleanup)
	})

	t.Run("nothing listening", func(t *testing.T) {
		conf := buildDeviceClientConfig(coreClosedURL(t), coreShortTimeoutMS, false, config.SerialConfig{})
		client, cleanup, err := validateAndConnect(conf)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Nil(t, cleanup)
	})

	t.Run("serial device missing", func(t *testing.T) {
		_, err := openDeviceLink("rtu://"+filepath.Join(t.TempDir(), "no-such-tty"), coreShortTimeoutMS, false, config.SerialConfig{Baud: 9600})
		require.ErrorIs(t, err, ErrTCPConnection)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("discovery connect", func(t *testing.T) {
		dev := mbtest.NewDevice().SetHolding(0, 0x1111)
		client, cleanup, err := connectDiscovery("tcp://"+dev.ServeTCP(t), false)
		require.NoError(t, err)
		defer cleanup()
		regs, err := client.ReadRegisters(context.Background(), 1, 0, 1, gomodbus.HoldingRegister)
		require.NoError(t, err)
		assert.Equal(t, []uint16{0x1111}, regs)
	})
}

func TestCoreParseUnitIDs(t *testing.T) {
	all, err := parseUnitIDs(" ALL ")
	require.NoError(t, err)
	require.Len(t, all, 255)
	assert.Equal(t, uint8(1), all[0])
	assert.Equal(t, uint8(255), all[254])

	got, err := ParseUnitIDs("7, 3-5 ,3")
	require.NoError(t, err)
	assert.Equal(t, []uint8{3, 4, 5, 7}, got)

	bad := []string{"", "  ", "1,,2", "-5", "5-", "a-5", "1-b", "0-5", "1-256", "9-2", "0", "256", "x", "1-2-3"}
	for _, in := range bad {
		t.Run("invalid "+in, func(t *testing.T) {
			_, err := ParseUnitIDs(in)
			require.Error(t, err)
			e, ok := errs.As(err)
			require.True(t, ok)
			assert.Equal(t, errs.KindInvalidInput, e.Kind)
			assert.Equal(t, errs.CodeInvalidUnitSelector, e.Code)
		})
	}
}

func TestCorePackCoilsToBytes(t *testing.T) {
	assert.Empty(t, packCoilsToBytes(nil))
	assert.Equal(t, []byte{0x01}, packCoilsToBytes([]bool{true}))
	assert.Equal(t, []byte{0x80, 0x01}, packCoilsToBytes([]bool{false, false, false, false, false, false, false, true, true}))
}

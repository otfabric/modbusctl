package modbus

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coreBlocksFile writes blocks as a JSON blocks file in the test's temporary directory.
func coreBlocksFile(t *testing.T, blocks ...types.AddressBlock) string {
	t.Helper()
	data, err := json.Marshal(blocks)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "blocks.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestCoreRecord_capturesEveryBlockEachIteration(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 9}, coreRange{100, 104})
	dc := coreTCP(t, dev)
	dc.Unit = 6
	out := coreCapturePath(t)

	var progress bytes.Buffer
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: dc, Timeout: coreTimeoutMS, Function: 3, Interval: 10, Duration: 60,
		BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 10}, types.AddressBlock{StartAddress: 100, RegisterCount: 5}),
		OutputFile: out,
	}, &progress)
	require.NoError(t, err)

	assert.Equal(t, dc.URL, sum.Target)
	assert.Equal(t, out, sum.CaptureOutputPath)
	assert.GreaterOrEqual(t, sum.Iterations, uint32(2))
	assert.LessOrEqual(t, sum.Iterations, uint32(7), "one iteration per 10 ms interval for 60 ms")
	assert.Equal(t, 2*int(sum.Iterations), sum.BlocksRecorded)
	assert.Equal(t, sum.BlocksRecorded, dev.RequestCount(3))

	records, header := coreLoadCapture(t, out)
	assert.Equal(t, uint8(6), header.Unit)
	assert.Equal(t, uint8(3), header.Function)
	require.Len(t, records, sum.BlocksRecorded)
	coreCapturedAddrs(t, records) // Checks every record against the device contents.
	for i, rec := range records {
		assert.Equal(t, uint32(i/2), rec.Iteration)
		if i%2 == 0 {
			assert.Equal(t, uint16(0), rec.StartAddress)
			assert.Equal(t, uint16(10), rec.RegisterCount)
		} else {
			assert.Equal(t, uint16(100), rec.StartAddress)
			assert.Equal(t, uint16(5), rec.RegisterCount)
		}
		if i > 0 {
			assert.GreaterOrEqual(t, rec.RequestTimestamp, records[i-1].ResponseTimestamp)
		}
	}

	text := progress.String()
	assert.Equal(t, int(sum.Iterations), strings.Count(text, "Recording "))
	assert.Contains(t, text, "Recording 0 started...\n")
	assert.Equal(t, int(sum.Iterations), strings.Count(text, "Recorded block: Start 0, Count 10\n"))
	assert.Equal(t, int(sum.Iterations), strings.Count(text, "Recorded block: Start 100, Count 5\n"))
	for _, req := range dev.Requests() {
		assert.Equal(t, uint8(6), req.Unit)
	}
}

// A block that cannot be read is reported and skipped; the others are still recorded.
func TestCoreRecord_unreadableBlockIsSkipped(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	out := coreCapturePath(t)
	var progress bytes.Buffer
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 4, Duration: 1,
		BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 500, RegisterCount: 2}, types.AddressBlock{StartAddress: 0, RegisterCount: 4}),
		OutputFile: out,
	}, &progress)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sum.Iterations, uint32(1))
	assert.Equal(t, int(sum.Iterations), sum.BlocksRecorded)

	records, header := coreLoadCapture(t, out)
	assert.Equal(t, uint8(4), header.Function)
	require.Len(t, records, sum.BlocksRecorded)
	for _, rec := range records {
		assert.Equal(t, uint16(0), rec.StartAddress)
	}
	assert.Equal(t, int(sum.Iterations), strings.Count(progress.String(), "Failed to read block (start: 500, count: 2): "))
	assert.Equal(t, 2*int(sum.Iterations), dev.RequestCount(4), "an illegal address is not retried")
}

// A busy device and a dropped connection are retried within the same iteration.
func TestCoreRecord_retriesBusyAndDroppedConnection(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	var n atomic.Int32
	dev.OnRequest = func(mbtest.Request) mbtest.Action {
		switch n.Add(1) {
		case 1:
			return mbtest.Action{Exception: mbtest.ExServerDeviceBusy}
		case 2:
			return mbtest.Action{Close: true}
		default:
			return mbtest.Action{}
		}
	}
	out := coreCapturePath(t)
	var progress bytes.Buffer
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Duration: 1, Debug: true,
		BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 4}),
		OutputFile: out,
	}, &progress)
	require.NoError(t, err)
	require.GreaterOrEqual(t, sum.Iterations, uint32(1))
	assert.Equal(t, int(sum.Iterations), sum.BlocksRecorded, "no block is lost")
	assert.NotContains(t, progress.String(), "Failed to read block")
	assert.Contains(t, progress.String(), "Retrying due to Modbus read exception on address 0 with count 4 (attempt 1)")
	assert.Contains(t, progress.String(), "Reconnecting due to connection error (attempt 2)")
	records, _ := coreLoadCapture(t, out)
	require.Len(t, records, sum.BlocksRecorded)
	assert.Equal(t, coreAddrSet(coreRange{0, 3}), coreCapturedAddrs(t, records))
}

// Duration 0 records nothing but still leaves a valid, empty capture.
func TestCoreRecord_zeroDuration(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	dir := t.TempDir()
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3,
		BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 4}),
		OutputFile: dir + "/",
	}, nil)
	require.NoError(t, err)
	assert.Zero(t, sum.Iterations)
	assert.Zero(t, sum.BlocksRecorded)
	assert.Zero(t, dev.RequestCount(0))
	assert.Equal(t, dir, filepath.Dir(sum.CaptureOutputPath))
	assert.Regexp(t, `^modbusctl_record_\d{8}_\d{6}\.mbcap$`, filepath.Base(sum.CaptureOutputPath))
	records, header := coreLoadCapture(t, sum.CaptureOutputPath)
	assert.Empty(t, records)
	assert.Equal(t, uint8(1), header.Unit, "the target is recorded even without samples")
}

func TestCoreRecord_setupFailures(t *testing.T) {
	t.Run("blocks file missing", func(t *testing.T) {
		out := coreCapturePath(t)
		_, err := RecordToCapture(context.Background(), config.RecordConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t)}, Timeout: coreShortTimeoutMS, Function: 3, Duration: 10,
			BlocksFile: filepath.Join(t.TempDir(), "nope.json"), OutputFile: out,
		}, nil)
		coreRequireKind(t, err, errs.KindOutput, errs.CodeInputFileOpenFailed)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.NoFileExists(t, out)
	})

	t.Run("blocks file is not JSON", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "blocks.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"start_address": 1}`), 0o600))
		_, err := RecordToCapture(context.Background(), config.RecordConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t)}, Timeout: coreShortTimeoutMS, Function: 3, Duration: 10,
			BlocksFile: path, OutputFile: coreCapturePath(t),
		}, nil)
		coreRequireKind(t, err, errs.KindOutput, errs.CodeInputDecodeFailed)
	})

	t.Run("connect refused", func(t *testing.T) {
		out := coreCapturePath(t)
		_, err := RecordToCapture(context.Background(), config.RecordConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t)}, Timeout: coreShortTimeoutMS, Function: 3, Duration: 10,
			BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 1}), OutputFile: out,
		}, nil)
		require.ErrorIs(t, err, ErrTCPConnection)
		assert.NoFileExists(t, out)
	})

	t.Run("output file cannot be created", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 3})
		_, err := RecordToCapture(context.Background(), config.RecordConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Duration: 10,
			BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 1}),
			OutputFile: filepath.Join(t.TempDir(), "missing", "record.mbcap"),
		}, nil)
		coreRequireKind(t, err, errs.KindOutput, errs.CodeOutputFileCreateFailed)
		assert.Zero(t, dev.RequestCount(0))
	})
}

// Cancellation during the interval between iterations ends the recording with the context
// error; what was recorded so far stays in the capture.
func TestCoreRecord_canceledBetweenIterations(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dev.OnRequest = func(mbtest.Request) mbtest.Action {
		// Cancel once the first block has been answered and recorded, well inside the
		// five-second interval that follows it. Cancelling right here would abort the
		// request in flight instead.
		time.AfterFunc(100*time.Millisecond, cancel)
		return mbtest.Action{}
	}
	out := coreCapturePath(t)
	start := time.Now()
	sum, err := RecordToCapture(ctx, config.RecordConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Interval: 5000, Duration: 60000,
		BlocksFile: coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 4}), OutputFile: out,
	}, nil)
	assert.Nil(t, sum)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, 1, dev.RequestCount(0))
	records, _ := coreLoadCapture(t, out)
	require.Len(t, records, 1)
	assert.Equal(t, coreAddrSet(coreRange{0, 3}), coreCapturedAddrs(t, records))
}

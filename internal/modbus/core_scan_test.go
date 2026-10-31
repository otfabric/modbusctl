// SPDX-License-Identifier: MIT

package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
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

// coreRange is an inclusive address range.
type coreRange struct{ lo, hi int }

// coreAddrSet returns the set of addresses in ranges.
func coreAddrSet(ranges ...coreRange) map[uint16]bool {
	set := map[uint16]bool{}
	for _, r := range ranges {
		for a := r.lo; a <= r.hi; a++ {
			set[uint16(a)] = true
		}
	}
	return set
}

// coreRegValue is the value test devices hold at addr: distinct per address so that a block
// read from the wrong place is noticed.
func coreRegValue(addr uint16) uint16 { return addr*7 + 3 }

// coreScanDevice returns a device whose holding and input registers are defined exactly in
// ranges, each holding coreRegValue of its address.
func coreScanDevice(ranges ...coreRange) *mbtest.Device {
	dev := mbtest.NewDevice()
	for a := range coreAddrSet(ranges...) {
		dev.SetHolding(a, coreRegValue(a))
		dev.SetInput(a, coreRegValue(a))
	}
	return dev
}

// coreCapturedAddrs checks every register record against the device contents and returns
// the set of addresses captured.
func coreCapturedAddrs(t *testing.T, records []types.CaptureRecord) map[uint16]bool {
	t.Helper()
	got := map[uint16]bool{}
	for i, rec := range records {
		require.Len(t, rec.Data, 2*int(rec.RegisterCount), "record %d", i)
		require.GreaterOrEqual(t, rec.ResponseTimestamp, rec.RequestTimestamp, "record %d", i)
		for j := 0; j < int(rec.RegisterCount); j++ {
			addr := rec.StartAddress + uint16(j)
			require.Equal(t, coreRegValue(addr), binary.BigEndian.Uint16(rec.Data[2*j:]), "record %d, address %d", i, addr)
			got[addr] = true
		}
	}
	return got
}

// coreGapMap is a register map with a small block, a gap, another small block, a large gap
// and a block longer than one maximum read.
var coreGapMap = []coreRange{{0, 9}, {20, 29}, {200, 329}}

func TestCoreScan_everyAlgorithm(t *testing.T) {
	tests := []struct {
		algo    string
		cfg     config.ScanConfig
		device  []coreRange
		want    []coreRange // Addresses that must be captured.
		exact   bool        // Nothing but want may be captured.
		banner  string
		maxReqs int
	}{
		{
			algo: "safe", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 399},
			device: coreGapMap, want: coreGapMap, exact: true,
			banner:  "Scanning registers from 0 to 399 with function code 3 (algo: safe)\nSafe algo: worst case with no hits = 400 addresses × 8 sizes = 3200 reads\n",
			maxReqs: 3200,
		},
		{
			algo: "smart", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 399},
			device: coreGapMap, want: coreGapMap, exact: true,
			banner:  "Scanning registers from 0 to 399 with function code 3 (algo: smart)\nSmart algo: worst case with no hits ≈ 2×400 − 4 = 796 reads\n",
			maxReqs: 796,
		},
		{
			algo: "deep", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 399},
			device: coreGapMap, want: coreGapMap, exact: true,
			banner:  "Scanning registers from 0 to 399 with function code 3 (algo: deep)\nDeep algo: worst case = phase 1 (smart) 796 reads + phase 2 up to 500 refinement = up to 1296 reads\n",
			maxReqs: 1296,
		},
		{
			// Stepped samples the map at every step and grows a block from each hit: it finds
			// something at 0, 200 and 300 but does not promise the whole map.
			algo: "stepped", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 399, Step: 100},
			device: coreGapMap, want: []coreRange{{0, 0}, {200, 200}, {300, 300}},
			banner:  "Scanning registers from 0 to 399 with function code 3 (algo: stepped)\nStepped algo (step=100): worst case with no hits = 4 steps × 6 probes = 24 reads\n",
			maxReqs: 4 * (6 + 6),
		},
		{
			// Linear reads whole 125-register blocks, so it suits a dense map.
			algo: "linear", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 499},
			device: []coreRange{{100, 349}}, want: []coreRange{{100, 349}}, exact: true,
			banner:  "Scanning registers from 0 to 499 with function code 3 (algo: linear)\nLinear algo: worst case with no hits = 4 probes (one per 125-block)\n",
			maxReqs: 30,
		},
		{
			// Boundary grows the block around the seed and nothing else.
			algo: "boundary", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 399, SeedStart: 205, SeedCount: 5},
			device: coreGapMap, want: []coreRange{{200, 329}}, exact: true,
			banner:  "Scanning registers from 0 to 399 with function code 3 (algo: boundary)\nBoundary algo: 1 seed + left/right expansion + binary search (depends on range)\n",
			maxReqs: 40,
		},
	}
	algos := map[string]bool{}
	for _, tc := range tests {
		algos[tc.algo] = true
		t.Run(tc.algo, func(t *testing.T) {
			dev := coreScanDevice(tc.device...)
			cfg := tc.cfg
			cfg.DeviceConfig = coreTCP(t, dev)
			cfg.Unit = 3
			cfg.Timeout = coreTimeoutMS
			cfg.Function = 3
			cfg.Algo = tc.algo
			cfg.OutputFile = coreCapturePath(t)

			var progress bytes.Buffer
			sum, err := ScanAndRecord(context.Background(), cfg, &progress)
			require.NoError(t, err)

			records, header := coreLoadCapture(t, cfg.OutputFile)
			assert.Equal(t, uint8(3), header.Unit)
			assert.Equal(t, uint8(3), header.Function)
			assert.Equal(t, "127.0.0.1", header.IP)

			got := coreCapturedAddrs(t, records)
			want := coreAddrSet(tc.want...)
			defined := coreAddrSet(tc.device...)
			for a := range want {
				assert.True(t, got[a], "address %d was not captured", a)
			}
			for a := range got {
				assert.True(t, defined[a], "address %d is not defined on the device", a)
				assert.True(t, int(a) >= int(cfg.StartAddress) && a <= cfg.EndAddress, "address %d is outside the scanned range", a)
				if tc.exact {
					assert.True(t, want[a], "address %d must not be captured", a)
				}
			}

			registers := 0
			for i, rec := range records {
				registers += int(rec.RegisterCount)
				assert.LessOrEqual(t, rec.RegisterCount, uint16(MaxBlockSize))
				if i > 0 {
					assert.GreaterOrEqual(t, rec.ResponseTimestamp, records[i-1].ResponseTimestamp, "samples are in time order")
				}
			}
			assert.Equal(t, cfg.URL, sum.Target)
			assert.Equal(t, tc.algo, sum.Algo)
			assert.Equal(t, cfg.OutputFile, sum.CaptureOutputPath)
			assert.Equal(t, dev.RequestCount(0), sum.TotalRequests)
			assert.Equal(t, dev.RequestCount(3), sum.TotalRequests, "only the configured function code is used")
			assert.LessOrEqual(t, sum.TotalRequests, tc.maxReqs)
			assert.Equal(t, sum.TotalRequests, sum.SuccessCount+sum.FailCount)
			assert.Equal(t, len(records), sum.SuccessCount)
			assert.Equal(t, len(records), sum.BlocksCaptured)
			assert.Equal(t, registers, sum.RegistersCaptured)
			assert.Equal(t, sum.FailCount, sum.ExceptionCount, "every failure is an illegal-address exception")
			assert.Zero(t, sum.TimeoutCount)
			assert.Zero(t, sum.TransportErrorCount)
			assert.NotNil(t, sum.Summary)
			_, err = time.ParseDuration(sum.Duration)
			assert.NoError(t, err, "duration %q", sum.Duration)

			text := progress.String()
			assert.True(t, strings.HasPrefix(text, tc.banner), "progress starts with:\n%s", text[:min(len(text), 300)])
			assert.Equal(t, sum.SuccessCount, strings.Count(text, "Block: Start: "))
			assert.NotContains(t, text, "DEBUG", "debug lines need --debug")
			for _, req := range dev.Requests() {
				assert.Equal(t, uint8(3), req.Unit)
			}
		})
	}
	for _, a := range config.ScanAlgorithms() {
		assert.True(t, algos[a] || a == string(config.ScanAlgoSunspec), "algorithm %q is not covered (sunspec has its own test)", a)
	}
}

// coreSunSpecDevice returns a device with a SunSpec map at base: the marker, one model of
// bodyLen registers and the end model. Every register holds coreRegValue except the headers.
func coreSunSpecDevice(base uint16, modelID, bodyLen uint16) (*mbtest.Device, []uint16) {
	regs := []uint16{0x5375, 0x6E53, modelID, bodyLen}
	for i := uint16(0); i < bodyLen; i++ {
		regs = append(regs, 0x1000+i)
	}
	regs = append(regs, 0xFFFF, 0x0000)
	return mbtest.NewDevice().SetHolding(base, regs...).SetInput(base, regs...), regs
}

// coreRecordsAsRegisters flattens records into address -> value.
func coreRecordsAsRegisters(t *testing.T, records []types.CaptureRecord) map[uint16]uint16 {
	t.Helper()
	out := map[uint16]uint16{}
	for _, rec := range records {
		require.Len(t, rec.Data, 2*int(rec.RegisterCount))
		for j := 0; j < int(rec.RegisterCount); j++ {
			out[rec.StartAddress+uint16(j)] = binary.BigEndian.Uint16(rec.Data[2*j:])
		}
	}
	return out
}

func TestCoreScan_sunspec(t *testing.T) {
	// The body is longer than one read, so it takes two.
	const bodyLen = 130
	tests := []struct {
		name       string
		cfg        config.ScanConfig
		wantBanner string
		wantFirst  uint16 // First address captured.
		wantReads  [][2]uint16
		wantFails  int
	}{
		{
			name:       "default bases",
			cfg:        config.ScanConfig{},
			wantBanner: "SunSpec discovery with function code 3 (algo: sunspec)\nSunSpec algo: worst case = 8 base probes + up to 256 models (header + body reads)\n",
			wantFirst:  40000,
			wantReads:  [][2]uint16{{40000, 2}, {40002, 2}, {40004, 125}, {40129, 5}, {40134, 2}},
			wantFails:  1, // Base 0 is probed first and is not readable.
		},
		{
			name:       "candidate bases",
			cfg:        config.ScanConfig{SunSpecBases: "100, 40000", SunSpecMaxModels: 4},
			wantBanner: "SunSpec discovery with function code 3 (algo: sunspec)\nSunSpec algo: worst case = 2 base probes + up to 4 models (header + body reads)\n",
			wantFirst:  40000,
			wantReads:  [][2]uint16{{40000, 2}, {40002, 2}, {40004, 125}, {40129, 5}, {40134, 2}},
			wantFails:  1,
		},
		{
			name:       "known base skips detection",
			cfg:        config.ScanConfig{SunSpecBase: 40000},
			wantBanner: "SunSpec discovery with function code 3 (algo: sunspec)\nSunSpec algo: worst case = 0 base probes + up to 256 models (header + body reads)\n",
			wantFirst:  40002,
			wantReads:  [][2]uint16{{40002, 2}, {40004, 125}, {40129, 5}, {40134, 2}},
		},
		{
			name:       "model limit",
			cfg:        config.ScanConfig{SunSpecBase: 40000, SunSpecMaxModels: 1},
			wantBanner: "SunSpec discovery with function code 3 (algo: sunspec)\nSunSpec algo: worst case = 0 base probes + up to 1 models (header + body reads)\n",
			wantFirst:  40002,
			wantReads:  [][2]uint16{{40002, 2}, {40004, 125}, {40129, 5}},
		},
		{
			name:       "span limit",
			cfg:        config.ScanConfig{SunSpecBase: 40000, SunSpecMaxSpan: 100},
			wantBanner: "SunSpec discovery with function code 3 (algo: sunspec)\n",
			wantFirst:  40002,
			wantReads:  [][2]uint16{{40002, 2}, {40004, 125}, {40129, 5}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dev, regs := coreSunSpecDevice(40000, 101, bodyLen)
			cfg := tc.cfg
			cfg.DeviceConfig = coreTCP(t, dev)
			cfg.Timeout = coreTimeoutMS
			cfg.Function = 3
			cfg.Algo = "sunspec"
			cfg.OutputFile = coreCapturePath(t)

			var progress bytes.Buffer
			sum, err := ScanAndRecord(context.Background(), cfg, &progress)
			require.NoError(t, err)
			assert.Equal(t, "sunspec", sum.Algo)
			assert.True(t, strings.HasPrefix(progress.String(), tc.wantBanner), "progress:\n%s", progress.String())

			records, _ := coreLoadCapture(t, cfg.OutputFile)
			var reads [][2]uint16
			for _, rec := range records {
				reads = append(reads, [2]uint16{rec.StartAddress, rec.RegisterCount})
			}
			assert.Equal(t, tc.wantReads, reads)
			for addr, v := range coreRecordsAsRegisters(t, records) {
				assert.Equal(t, regs[addr-40000], v, "address %d", addr)
			}
			assert.Equal(t, len(tc.wantReads), sum.SuccessCount)
			assert.Equal(t, tc.wantFails, sum.FailCount)
			assert.Equal(t, tc.wantFails, sum.ExceptionCount)
			assert.Equal(t, len(tc.wantReads)+tc.wantFails, dev.RequestCount(3))
		})
	}

	t.Run("no marker anywhere", func(t *testing.T) {
		dev := coreScanDevice(coreRange{40000, 40001})
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 4, Algo: "SunSpec", OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 8, sum.TotalRequests, "one probe per default base")
		assert.Equal(t, 8, dev.RequestCount(4))
		// A readable base without the marker is still a successful read and is captured.
		assert.Equal(t, 1, sum.SuccessCount)
		assert.Equal(t, 7, sum.ExceptionCount)
		records, header := coreLoadCapture(t, out)
		require.Len(t, records, 1)
		assert.Equal(t, uint16(40000), records[0].StartAddress)
		assert.Equal(t, uint8(4), header.Function)
	})
}

func TestCoreScan_inputRegistersAndCoils(t *testing.T) {
	t.Run("input registers", func(t *testing.T) {
		dev := mbtest.NewDevice().SetInput(10, 0xAAAA, 0xBBBB, 0xCCCC).SetHolding(0, 1, 2, 3, 4)
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 4, StartAddress: 8, EndAddress: 15, Algo: "smart", OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 3, sum.RegistersCaptured)
		records, header := coreLoadCapture(t, out)
		assert.Equal(t, uint8(4), header.Function)
		assert.Equal(t, map[uint16]uint16{10: 0xAAAA, 11: 0xBBBB, 12: 0xCCCC}, coreRecordsAsRegisters(t, records))
		assert.Zero(t, dev.RequestCount(3))
	})

	t.Run("coils are captured as packed bits", func(t *testing.T) {
		dev := mbtest.NewDevice().SetCoils(0, true, false, true, true, false, false, false, false, true, true)
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 1, StartAddress: 0, EndAddress: 9, Algo: "linear", OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, sum.SuccessCount)
		assert.Equal(t, 10, sum.RegistersCaptured)
		records, header := coreLoadCapture(t, out)
		assert.Equal(t, uint8(1), header.Function)
		require.Len(t, records, 1)
		assert.Equal(t, uint16(10), records[0].RegisterCount)
		assert.Equal(t, []byte{0x0D, 0x03}, records[0].Data)
	})
}

func TestCoreScan_debugProgress(t *testing.T) {
	dev := coreScanDevice(coreRange{2, 3})
	var progress bytes.Buffer
	sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 3,
		Algo: "smart", Debug: true, OutputFile: coreCapturePath(t),
	}, &progress)
	require.NoError(t, err)
	// [0,3] fails, [0,1] fails, 0 and 1 fail, [2,3] succeeds.
	assert.Equal(t, 5, sum.TotalRequests)
	assert.Equal(t, 1, sum.SuccessCount)

	text := progress.String()
	assert.Contains(t, text, "DEBUG [smart] init: range [0,3] initial intervals=1\n")
	assert.Contains(t, text, "DEBUG [exec] next task: start=0 count=4 end=3\n")
	assert.Contains(t, text, "DEBUG [exec] result: exception code=0x02 (start=0 count=4)\n")
	assert.Contains(t, text, "DEBUG [smart] split fail: [0,3] -> left [0,1] right [2,3]")
	assert.Contains(t, text, "DEBUG [exec] result: success (start=2 count=2)\n")
	assert.Contains(t, text, "Block: Start: 2, End: 3, Count: 2\n")
	assert.Equal(t, 5, strings.Count(text, "DEBUG [exec] next task:"))
	assert.Equal(t, 5, strings.Count(text, "DEBUG [exec] result:"))
}

// Every strategy explains itself under --debug and stays silent otherwise.
func TestCoreScan_strategyDebugOutput(t *testing.T) {
	configs := map[string]config.ScanConfig{
		"safe":     {StartAddress: 0, EndAddress: 40},
		"smart":    {StartAddress: 0, EndAddress: 40},
		"deep":     {StartAddress: 0, EndAddress: 40},
		"stepped":  {StartAddress: 0, EndAddress: 40, Step: 10, StepHalfOffset: true},
		"linear":   {StartAddress: 0, EndAddress: 400},
		"boundary": {StartAddress: 0, EndAddress: 40, SeedStart: 12, SeedCount: 2},
	}
	readable := func(a int) bool { return a >= 10 && a <= 300 }
	for algo, cfg := range configs {
		t.Run(algo, func(t *testing.T) {
			cfg.Algo = algo

			var quiet bytes.Buffer
			coreSimulate(t, scanSettings{ScanConfig: cfg, DebugWriter: &quiet}, readable)
			assert.Empty(t, quiet.String())

			cfg.Debug = true
			var debug bytes.Buffer
			tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg, DebugWriter: &debug}, readable)
			require.NotEmpty(t, tasks)
			assert.Contains(t, debug.String(), "DEBUG ["+algo+"] ")

			// Debug without a writer must not panic and must not change the result.
			silent, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, readable)
			assert.Equal(t, tasks, silent)
		})
	}
}

func TestCoreScan_setupFailures(t *testing.T) {
	t.Run("connect refused", func(t *testing.T) {
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t), Unit: 1}, Timeout: coreShortTimeoutMS,
			Function: 3, EndAddress: 10, OutputFile: out,
		}, nil)
		assert.Nil(t, sum)
		require.ErrorIs(t, err, ErrTCPConnection)
		assert.NoFileExists(t, out, "no capture is created without a connection")
	})

	t.Run("output file cannot be created", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 10})
		_, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, EndAddress: 10,
			OutputFile: filepath.Join(t.TempDir(), "missing", "scan.mbcap"),
		}, nil)
		coreRequireKind(t, err, errs.KindOutput, errs.CodeOutputFileCreateFailed)
		assert.Zero(t, dev.RequestCount(0))
	})

	t.Run("unknown algorithm", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 10})
		var progress bytes.Buffer
		_, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, EndAddress: 10,
			Algo: "quantum", OutputFile: coreCapturePath(t),
		}, &progress)
		coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
		assert.Contains(t, err.Error(), `unsupported scan algorithm "quantum"`)
		assert.Zero(t, dev.RequestCount(0))
		assert.Empty(t, progress.String())
	})
}

// An output path ending in a slash is a directory; the algorithm defaults to safe.
func TestCoreScan_outputDirectoryAndDefaultAlgorithm(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 4})
	dir := t.TempDir()
	sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 4,
		OutputFile: dir + "/",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "safe", sum.Algo)
	assert.Equal(t, dir, filepath.Dir(sum.CaptureOutputPath))
	assert.Regexp(t, `^modbusctl_scan_\d{8}_\d{6}\.mbcap$`, filepath.Base(sum.CaptureOutputPath))
	records, _ := coreLoadCapture(t, sum.CaptureOutputPath)
	assert.Equal(t, coreAddrSet(coreRange{0, 4}), coreCapturedAddrs(t, records))
}

// The normalized algorithm set by validation wins over the raw flag value.
func TestCoreScan_normalizedAlgorithm(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 4})
	sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 4,
		Algo: " SMART ", NormalizedAlgo: config.ScanAlgoLinear, OutputFile: coreCapturePath(t),
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "linear", sum.Algo)
	assert.Equal(t, 1, sum.TotalRequests, "linear reads the whole range at once")
}

func TestCoreScan_timeouts(t *testing.T) {
	t.Run("counted as timeouts", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "linear", OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, sum.TotalRequests)
		assert.Equal(t, 1, sum.FailCount)
		assert.Equal(t, 1, sum.TimeoutCount)
		assert.Zero(t, sum.ExceptionCount)
		assert.Zero(t, sum.TransportErrorCount)
		assert.Zero(t, sum.AvgResponseMs)
		records, _ := coreLoadCapture(t, out)
		assert.Empty(t, records)
	})

	t.Run("retried once when configured", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		dev.OnRequest = coreFirstN(1, mbtest.Action{Drop: true})
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "linear", RetryOnTimeoutTransport: 1, OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, sum.TotalRequests, "the retry counts as a request")
		assert.Equal(t, 2, dev.RequestCount(3))
		assert.Equal(t, 1, sum.SuccessCount)
		assert.Zero(t, sum.FailCount)
		assert.Zero(t, sum.TimeoutCount)
		records, _ := coreLoadCapture(t, out)
		assert.Equal(t, coreAddrSet(coreRange{0, 9}), coreCapturedAddrs(t, records))
	})

	t.Run("retry that times out again", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
		start := time.Now()
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "linear", RetryOnTimeoutTransport: 1, Delay: 30, OutputFile: coreCapturePath(t),
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, sum.TotalRequests)
		assert.Equal(t, 2, dev.RequestCount(3))
		assert.Equal(t, 1, sum.FailCount)
		assert.Equal(t, 1, sum.TimeoutCount)
		assert.GreaterOrEqual(t, time.Since(start), 2*coreShortTimeoutMS*time.Millisecond)
	})

	t.Run("exceptions are not retried", func(t *testing.T) {
		dev := coreScanDevice(coreRange{50, 59})
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "linear", RetryOnTimeoutTransport: 1, OutputFile: coreCapturePath(t),
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 1, sum.TotalRequests)
		assert.Equal(t, 1, dev.RequestCount(3))
		assert.Equal(t, 1, sum.ExceptionCount)
	})
}

func TestCoreScan_connectionClosedByDevice(t *testing.T) {
	// The failed block is split and read again over a new connection.
	t.Run("scan continues after re-dialing", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 3})
		dev.OnRequest = coreFirstN(1, mbtest.Action{Close: true})
		out := coreCapturePath(t)
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 3,
			Algo: "smart", OutputFile: out,
		}, nil)
		require.NoError(t, err)
		assert.Equal(t, 3, sum.TotalRequests)
		assert.Equal(t, 1, sum.FailCount)
		assert.Equal(t, 1, sum.TransportErrorCount)
		assert.Zero(t, sum.ExceptionCount)
		assert.Zero(t, sum.TimeoutCount)
		assert.Equal(t, 2, sum.SuccessCount)
		records, _ := coreLoadCapture(t, out)
		assert.Equal(t, coreAddrSet(coreRange{0, 3}), coreCapturedAddrs(t, records))
	})

	t.Run("retried once when configured", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 3})
		dev.OnRequest = coreFirstN(1, mbtest.Action{Close: true})
		var progress bytes.Buffer
		sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 3,
			Algo: "smart", RetryOnTimeoutTransport: 1, Debug: true, OutputFile: coreCapturePath(t),
		}, &progress)
		require.NoError(t, err)
		assert.Equal(t, 2, sum.TotalRequests)
		assert.Equal(t, 1, sum.SuccessCount)
		assert.Zero(t, sum.FailCount)
		assert.Zero(t, sum.TransportErrorCount)
		assert.Equal(t, 4, sum.RegistersCaptured)
		assert.Contains(t, progress.String(), "Reconnecting due to connection error (attempt 1)")
	})
}

// A failure that is neither an exception, a timeout nor a transport error is only counted
// as a failure.
func TestCoreScan_unclassifiedFailure(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 9})
	var progress bytes.Buffer
	sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 9, StartAddress: 0, EndAddress: 9,
		Algo: "linear", Debug: true, OutputFile: coreCapturePath(t),
	}, &progress)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.TotalRequests)
	assert.Equal(t, 1, sum.FailCount)
	assert.Zero(t, sum.ExceptionCount)
	assert.Zero(t, sum.TimeoutCount)
	assert.Zero(t, sum.TransportErrorCount)
	assert.Zero(t, dev.RequestCount(0), "an unsupported function code never reaches the device")
	assert.Contains(t, progress.String(), "DEBUG [exec] result: unknown (start=0 count=10)\n")
}

func TestCoreScan_delayBetweenRequests(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	dev.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Delay: 3 * time.Millisecond} }
	start := time.Now()
	sum, err := ScanAndRecord(context.Background(), config.ScanConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 7,
		Algo: "smart", Delay: 20, OutputFile: coreCapturePath(t),
	}, nil)
	require.NoError(t, err)
	// [0,7] fails, [0,3] succeeds, [4,7] is split down to single registers: 1 + 1 + 7 requests.
	assert.Equal(t, 9, sum.TotalRequests)
	// The delay is applied before and after every request.
	assert.GreaterOrEqual(t, time.Since(start), time.Duration(sum.TotalRequests)*2*20*time.Millisecond)
	assert.GreaterOrEqual(t, sum.AvgResponseMs, int64(3))
	d, err := time.ParseDuration(sum.Duration)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, d, time.Duration(sum.TotalRequests)*2*20*time.Millisecond)
}

func TestCoreScan_cancellation(t *testing.T) {
	t.Run("before the first request", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		sum, err := ScanAndRecord(ctx, config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "safe", Delay: 500, OutputFile: coreCapturePath(t),
		}, nil)
		assert.Nil(t, sum)
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, dev.RequestCount(0))
	})

	t.Run("between requests", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var n atomic.Int32
		dev.OnRequest = func(mbtest.Request) mbtest.Action {
			if n.Add(1) == 3 {
				cancel()
			}
			return mbtest.Action{}
		}
		start := time.Now()
		sum, err := ScanAndRecord(ctx, config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 2000,
			Algo: "safe", Delay: 5, OutputFile: coreCapturePath(t),
		}, nil)
		assert.Nil(t, sum)
		require.ErrorIs(t, err, context.Canceled)
		assert.LessOrEqual(t, dev.RequestCount(0), 4, "the scan stops at once")
		assert.Less(t, time.Since(start), time.Second)
	})

	// The timeout is retried after the configured delay; cancellation ends that wait.
	t.Run("while waiting to retry", func(t *testing.T) {
		dev := coreScanDevice(coreRange{0, 9})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dev.OnRequest = func(mbtest.Request) mbtest.Action {
			time.AfterFunc((coreShortTimeoutMS+50)*time.Millisecond, cancel)
			return mbtest.Action{Drop: true}
		}
		sum, err := ScanAndRecord(ctx, config.ScanConfig{
			DeviceConfig: coreTCP(t, dev), Timeout: coreShortTimeoutMS, Function: 3, StartAddress: 0, EndAddress: 9,
			Algo: "linear", RetryOnTimeoutTransport: 1, Delay: 400, OutputFile: coreCapturePath(t),
		}, nil)
		assert.Nil(t, sum)
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, dev.RequestCount(0), "the retry is never sent")
	})
}

func TestCorePrintScanWorstCaseHint(t *testing.T) {
	tests := []struct {
		name string
		algo string
		cfg  config.ScanConfig
		want string
	}{
		{name: "safe", algo: "safe", cfg: config.ScanConfig{StartAddress: 10, EndAddress: 19}, want: "Safe algo: worst case with no hits = 10 addresses × 8 sizes = 80 reads\n"},
		{name: "safe, inverted range", algo: "safe", cfg: config.ScanConfig{StartAddress: 20, EndAddress: 19}, want: "Safe algo: worst case with no hits = 0 addresses × 8 sizes = 0 reads\n"},
		{name: "smart", algo: "smart", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 124}, want: "Smart algo: worst case with no hits ≈ 2×125 − 1 = 249 reads\n"},
		{name: "smart, full address space", algo: "smart", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 65535}, want: "Smart algo: worst case with no hits ≈ 2×65536 − 525 = 130547 reads\n"},
		{name: "deep", algo: "deep", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 249}, want: "Deep algo: worst case = phase 1 (smart) 498 reads + phase 2 up to 500 refinement = up to 998 reads\n"},
		{name: "stepped", algo: "stepped", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 2500, Step: 1000}, want: "Stepped algo (step=1000): worst case with no hits = 3 steps × 6 probes = 18 reads\n"},
		{name: "stepped, inverted range", algo: "stepped", cfg: config.ScanConfig{StartAddress: 5, EndAddress: 1, Step: 10}, want: "Stepped algo (step=10): worst case with no hits = 0 steps × 6 probes = 0 reads\n"},
		{name: "linear", algo: "linear", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 125}, want: "Linear algo: worst case with no hits = 2 probes (one per 125-block)\n"},
		{name: "linear, inverted range", algo: "linear", cfg: config.ScanConfig{StartAddress: 9, EndAddress: 1}, want: "Linear algo: worst case with no hits = 0 probes (one per 125-block)\n"},
		{name: "boundary", algo: "boundary", cfg: config.ScanConfig{}, want: "Boundary algo: 1 seed + left/right expansion + binary search (depends on range)\n"},
		{name: "sunspec defaults", algo: "sunspec", cfg: config.ScanConfig{}, want: "SunSpec algo: worst case = 8 base probes + up to 256 models (header + body reads)\n"},
		{name: "sunspec candidate bases", algo: "sunspec", cfg: config.ScanConfig{SunSpecBases: "0,40000,50000", SunSpecMaxModels: 10}, want: "SunSpec algo: worst case = 3 base probes + up to 10 models (header + body reads)\n"},
		{name: "sunspec unparsable bases fall back to the defaults", algo: "sunspec", cfg: config.ScanConfig{SunSpecBases: "abc"}, want: "SunSpec algo: worst case = 8 base probes + up to 256 models (header + body reads)\n"},
		{name: "sunspec known base", algo: "sunspec", cfg: config.ScanConfig{SunSpecBase: 40000, SunSpecBases: "0,1"}, want: "SunSpec algo: worst case = 0 base probes + up to 256 models (header + body reads)\n"},
		{name: "unknown algorithm prints nothing", algo: "quantum", cfg: config.ScanConfig{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printScanWorstCaseHint(tc.cfg, tc.algo, &out)
			assert.Equal(t, tc.want, out.String())
		})
	}
}

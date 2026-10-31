// SPDX-License-Identifier: MIT

package modbus

// End-to-end tests of the capture pipeline: scan a device, turn the scan into
// a blocks file, record the device with it, and serve the recording back,
// static and replayed, over every network transport the device can be
// reached on. The device's register 100 counts the reads it answers, so the
// order and completeness of the recorded and replayed values can be checked
// exactly.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/analysis"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// e2eCountingDevice returns a device with holding registers 100-104 and 200-201
// and coils 0-7. Register 100 holds the number of FC03 reads of it the device
// has answered (1 for the first answer); the others are constant.
func e2eCountingDevice() (*mbtest.Device, func() uint16) {
	d := mbtest.NewDevice().
		SetHolding(100, 0, 0xBEEF, 0x1234, 0x0001, 0xFFFF).
		SetHolding(200, 7, 8).
		SetCoils(0, true, false, true, true, false, false, true, false)
	var mu sync.Mutex
	var n uint16
	d.OnRequest = func(r mbtest.Request) mbtest.Action {
		if len(r.PDU) == 5 && r.PDU[0] == 3 && binary.BigEndian.Uint16(r.PDU[1:]) <= 100 &&
			binary.BigEndian.Uint16(r.PDU[1:])+binary.BigEndian.Uint16(r.PDU[3:]) > 100 {
			mu.Lock()
			n++
			d.SetHolding(100, n)
			mu.Unlock()
		}
		return mbtest.Action{}
	}
	return d, func() uint16 { mu.Lock(); defer mu.Unlock(); return n }
}

type e2eTransport struct {
	name  string
	serve func(d *mbtest.Device, t testing.TB) string
}

var e2eTransports = []e2eTransport{
	{"tcp", func(d *mbtest.Device, t testing.TB) string { return "tcp://" + d.ServeTCP(t) }},
	{"rtuovertcp", func(d *mbtest.Device, t testing.TB) string { return "rtuovertcp://" + d.ServeRTUOverTCP(t) }},
	{"asciiovertcp", func(d *mbtest.Device, t testing.TB) string { return "asciiovertcp://" + d.ServeASCIIOverTCP(t) }},
	{"udp", func(d *mbtest.Device, t testing.TB) string { return "udp://" + d.ServeUDP(t) }},
}

// e2eRecord records the blocks of d through url for duration and returns the
// capture path and summary.
func e2eRecord(t *testing.T, url string, blocks []types.AddressBlock, fc uint8, interval, duration uint32, exceptions bool) (string, *types.RecordSummaryResult) {
	t.Helper()
	dir := t.TempDir()
	bf := filepath.Join(dir, "blocks.json")
	data, err := json.Marshal(blocks)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(bf, data, 0o600))
	out := filepath.Join(dir, "record.mbcap")
	sum, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: url, Unit: 1}, Timeout: coreTimeoutMS, Function: fc,
		Interval: interval, Duration: duration, BlocksFile: bf, OutputFile: out, Exceptions: exceptions,
	}, nil)
	require.NoError(t, err)
	return out, sum
}

// e2eCounterValues returns the register-100 values of the FC03 samples of a capture, in file order.
func e2eCounterValues(t *testing.T, l *capfile.Loaded) []uint16 {
	t.Helper()
	var out []uint16
	for i := range l.Samples {
		av, ok := capture.SplitAddresses(&l.Samples[i])
		if ok && av.Function == 3 && av.Start <= 100 && int(av.Start)+len(av.Values) > 100 {
			out = append(out, av.Values[100-av.Start])
		}
	}
	return out
}

func requireStrictlyIncreasing(t *testing.T, vals []uint16, what string) {
	t.Helper()
	for i := 1; i < len(vals); i++ {
		require.Greater(t, vals[i], vals[i-1], "%s: value %d (%d) after %d: a duplicate or stale sample", what, i, vals[i], vals[i-1])
	}
}

func TestE2ECapture_scanExtractRecordServe(t *testing.T) {
	for _, tr := range e2eTransports {
		t.Run(tr.name, func(t *testing.T) {
			dev, answered := e2eCountingDevice()
			url := tr.serve(dev, t)

			// 1. Scan the device: the scan finds the two holding-register blocks.
			scanOut := filepath.Join(t.TempDir(), "scan.mbcap")
			_, err := ScanAndRecord(context.Background(), config.ScanConfig{
				DeviceConfig: config.DeviceConfig{URL: url, Unit: 1}, Timeout: coreTimeoutMS, Function: 3,
				StartAddress: 90, EndAddress: 210, Algo: "smart", OutputFile: scanOut,
			}, nil)
			require.NoError(t, err)

			// 2. The scan becomes a blocks file.
			var blocksJSON bytes.Buffer
			require.NoError(t, analysis.ExportAddressBlocks(&blocksJSON, scanOut, config.CaptureSelection{}))
			var blocks []types.AddressBlock
			require.NoError(t, json.Unmarshal(blocksJSON.Bytes(), &blocks))
			covered := map[uint16]bool{}
			for _, b := range blocks {
				for a := b.StartAddress; a < b.StartAddress+b.RegisterCount; a++ {
					covered[a] = true
				}
			}
			for _, a := range []uint16{100, 101, 102, 103, 104, 200, 201} {
				require.True(t, covered[a], "the scan missed register %d: %+v", a, blocks)
			}

			// 3. Record with the blocks file.
			before := answered()
			rec, sum := e2eRecord(t, url, blocks, 3, 20, 400, false)
			l := capfiletest.Load(t, rec)
			require.False(t, l.Truncated)
			require.True(t, l.Capture.End.Known)
			require.Len(t, l.Samples, sum.BlocksRecorded)
			require.Equal(t, len(blocks)*int(sum.Iterations), sum.BlocksRecorded, "every block of every iteration")
			counter := e2eCounterValues(t, l)
			require.Len(t, counter, int(sum.Iterations))
			requireStrictlyIncreasing(t, counter, "recorded")
			require.Equal(t, before+1, counter[0])
			require.Equal(t, answered(), counter[len(counter)-1], "the last answer was recorded")
			for i := range l.Samples {
				require.Equal(t, uint8(3), l.Samples[i].Function())
				require.False(t, l.Samples[i].IsException())
			}
			tgt := l.Targets[0]
			require.Equal(t, url, tgt.Tags["url"])

			// 4. Static: the last recorded values.
			srv, _ := staticServer(t, config.StaticServerConfig{InputFile: rec})
			c := srv.client(t)
			got, err := c.ReadHoldingRegisters(context.Background(), 1, 100, 5)
			require.NoError(t, err)
			require.Equal(t, []uint16{counter[len(counter)-1], 0xBEEF, 0x1234, 0x0001, 0xFFFF}, got)
			got, err = c.ReadHoldingRegisters(context.Background(), 1, 200, 2)
			require.NoError(t, err)
			require.Equal(t, []uint16{7, 8}, got)

			// 5. Replay once at 4x: register 100 walks through the recorded values in order
			// and stays on the last one.
			var progress collSyncBuffer
			rs := collStartServer(t, func(ctx context.Context, a collAddr) error {
				return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: rec, Speed: "4", Loops: 1}, a.Progress(&progress))
			})
			rc := rs.client(t)
			recorded := map[uint16]bool{}
			for _, v := range counter {
				recorded[v] = true
			}
			var served []uint16
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				got, err := rc.ReadHoldingRegisters(context.Background(), 1, 100, 2)
				require.NoError(t, err)
				require.True(t, recorded[got[0]], "replay served %d, which was never recorded", got[0])
				require.Equal(t, uint16(0xBEEF), got[1])
				if len(served) == 0 || served[len(served)-1] != got[0] {
					served = append(served, got[0])
				}
				if got[0] == counter[len(counter)-1] {
					break
				}
				time.Sleep(time.Millisecond)
			}
			require.Equal(t, counter[len(counter)-1], served[len(served)-1], "replay never reached the last value: %v", served)
			requireStrictlyIncreasing(t, served, "replayed")
			require.ErrorIs(t, rs.stop(t), context.Canceled)
			assert.Contains(t, progress.String(), "📊 Served ")
		})
	}
}

// Every lossless encoding of a recording serves exactly like the binary file.
func TestE2ECapture_encodingsServeTheSame(t *testing.T) {
	dev, _ := e2eCountingDevice()
	rec, _ := e2eRecord(t, "tcp://"+dev.ServeTCP(t), []types.AddressBlock{{StartAddress: 100, RegisterCount: 5}}, 3, 10, 120, false)
	coils, _ := e2eRecord(t, "tcp://"+dev.ServeTCP(t), []types.AddressBlock{{StartAddress: 0, RegisterCount: 8}}, 1, 10, 60, false)
	read := func(path string) (regs []uint16, bits []bool) {
		srv, _ := staticServer(t, config.StaticServerConfig{InputFile: path, At: "50ms"})
		c := srv.client(t)
		var err error
		regs, err = c.ReadHoldingRegisters(context.Background(), 1, 100, 5)
		if err != nil {
			bits, err = c.ReadCoils(context.Background(), 1, 0, 8)
		}
		require.NoError(t, err, path)
		return regs, bits
	}
	for _, src := range []string{rec, coils} {
		wantRegs, wantBits := read(src)
		for _, f := range []string{"jsonl", "csv"} {
			out := filepath.Join(t.TempDir(), "c.mbcap."+f)
			rtConvert(t, src, out)
			regs, bits := read(out)
			require.Equal(t, wantRegs, regs, f)
			require.Equal(t, wantBits, bits, f)
		}
	}
}

// A device that answers some reads with an exception: with --record-exceptions
// the exceptions are in the capture, and the server still serves the values.
func TestE2ECapture_recordExceptions(t *testing.T) {
	dev, _ := e2eCountingDevice()
	inner := dev.OnRequest
	var mu sync.Mutex
	k := 0
	dev.OnRequest = func(r mbtest.Request) mbtest.Action {
		mu.Lock()
		k++
		every3 := k%3 == 0
		mu.Unlock()
		if every3 {
			return mbtest.Action{Exception: mbtest.ExIllegalDataAddress}
		}
		return inner(r)
	}
	rec, sum := e2eRecord(t, "tcp://"+dev.ServeTCP(t), []types.AddressBlock{{StartAddress: 100, RegisterCount: 2}}, 3, 5, 200, true)
	l := capfiletest.Load(t, rec)
	require.True(t, l.Header.Exceptions)
	exc := 0
	for i := range l.Samples {
		if l.Samples[i].IsException() {
			exc++
			require.Equal(t, []byte{0x83, 0x02}, l.Samples[i].Response)
		}
	}
	require.Positive(t, exc)
	require.Equal(t, sum.BlocksRecorded, len(l.Samples)-exc, "the summary counts the blocks read")
	requireStrictlyIncreasing(t, e2eCounterValues(t, l), "recorded")

	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: rec})
	got, err := srv.client(t).ReadHoldingRegisters(context.Background(), 1, 100, 2)
	require.NoError(t, err, "recorded exceptions are not served")
	require.Equal(t, uint16(0xBEEF), got[1])
}

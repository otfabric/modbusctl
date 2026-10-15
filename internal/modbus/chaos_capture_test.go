package modbus

// Chaos tests of recording and serving captures: a device that randomly
// answers busy, drops requests, closes connections and answers late; clients
// that misbehave against a server; and damaged capture files. They check that
// nothing panics or hangs, that a capture never holds a duplicate, stale or
// failed read, and that well-behaved clients keep getting correct answers.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/modbusctl/internal/analysis"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/replay"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

// chaosDevice is e2eCountingDevice with faults: each request is, at random,
// answered busy, dropped, answered after its connection is closed, delayed, or
// answered normally. Only normal answers count. It returns the set of counter
// values the device actually answered with.
func chaosDevice(seed int64, stream bool) (*mbtest.Device, func() map[uint16]bool) {
	d := mbtest.NewDevice().SetHolding(100, 0, 0xBEEF, 0x1234)
	rng := rand.New(rand.NewSource(seed))
	var mu sync.Mutex
	var n uint16
	answered := map[uint16]bool{}
	d.OnRequest = func(mbtest.Request) mbtest.Action {
		mu.Lock()
		defer mu.Unlock()
		switch x := rng.Intn(100); {
		case x < 15:
			return mbtest.Action{Exception: mbtest.ExServerDeviceBusy}
		case x < 22:
			return mbtest.Action{Drop: true}
		case x < 27 && stream:
			return mbtest.Action{Close: true}
		case x < 31:
			// An answer the client rejects: one register short, or the wrong function.
			// The device did answer, but nothing usable: it must not be recorded.
			if x%2 == 0 {
				return mbtest.Action{Reply: []byte{0x03, 0x04, 0x00, 0x01, 0xBE, 0xEF}}
			}
			return mbtest.Action{Reply: []byte{0x04, 0x06, 0, 1, 0xBE, 0xEF, 0x12, 0x34}}
		case x < 35:
			// Late, but within the client timeout: still a valid answer.
			n++
			d.SetHolding(100, n)
			answered[n] = true
			return mbtest.Action{Delay: 20 * time.Millisecond}
		}
		n++
		d.SetHolding(100, n)
		answered[n] = true
		return mbtest.Action{}
	}
	return d, func() map[uint16]bool {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[uint16]bool, len(answered))
		for k := range answered {
			out[k] = true
		}
		return out
	}
}

// Recording a misbehaving device keeps only valid answers: no exception, no
// duplicate from a retried attempt, no stale answer, and nothing the device did
// not send.
func TestChaosCapture_recordFlakyDevice(t *testing.T) {
	transports := []struct {
		name   string
		stream bool
		serve  func(d *mbtest.Device, t testing.TB) string
	}{
		{"tcp", true, func(d *mbtest.Device, t testing.TB) string { return "tcp://" + d.ServeTCP(t) }},
		{"rtuovertcp", true, func(d *mbtest.Device, t testing.TB) string { return "rtuovertcp://" + d.ServeRTUOverTCP(t) }},
		{"udp", false, func(d *mbtest.Device, t testing.TB) string { return "udp://" + d.ServeUDP(t) }},
	}
	for _, tr := range transports {
		for _, exceptions := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/exceptions=%v", tr.name, exceptions), func(t *testing.T) {
				t.Parallel()
				dev, answered := chaosDevice(int64(len(tr.name))*7+1, tr.stream)
				url := tr.serve(dev, t)
				dir := t.TempDir()
				bf := filepath.Join(dir, "blocks.json")
				require.NoError(t, os.WriteFile(bf, []byte(`[{"start_address":100,"register_count":3}]`), 0o600))
				out := filepath.Join(dir, "chaos.mbcap")
				sum, err := RecordToCapture(context.Background(), config.RecordConfig{
					DeviceConfig: config.DeviceConfig{URL: url, Unit: 1}, Timeout: 120, Function: 3,
					Interval: 2, Duration: 1500, BlocksFile: bf, OutputFile: out, Exceptions: exceptions,
				}, nil)
				require.NoError(t, err)

				l := capfiletest.Load(t, out)
				require.False(t, l.Truncated, "the capture is complete")
				require.NoError(t, l.Validate())
				vals := e2eCounterValues(t, l)
				require.Equal(t, sum.BlocksRecorded, len(vals))
				require.Greater(t, len(vals), 20, "the recording made progress despite the faults")
				requireStrictlyIncreasing(t, vals, "recorded")
				sent := answered()
				for _, v := range vals {
					require.True(t, sent[v], "recorded %d, which the device never answered", v)
				}
				exc := 0
				for i := range l.Samples {
					s := &l.Samples[i]
					if s.IsException() {
						exc++
						require.Equal(t, []byte{0x83, mbtest.ExServerDeviceBusy}, s.Response)
						continue
					}
					av, ok := capture.SplitAddresses(s)
					require.True(t, ok)
					require.Equal(t, []uint16{0xBEEF, 0x1234}, av.Values[1:])
				}
				if exceptions {
					require.Positive(t, exc, "busy answers were recorded")
				} else {
					require.Zero(t, exc)
				}
			})
		}
	}
}

// chaosReplayCapture is a capture of one target whose register 100 counts up
// every 5 ms for 2 s; register 101 is constant.
func chaosReplayCapture(t *testing.T) string {
	c := &capture.Capture{
		Header:  capture.Header{TickNS: 1000, WallStart: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)},
		Targets: []capture.Target{{ID: 1, Unit: 1}},
		Streams: []capture.Stream{{ID: 1, Target: 1, Role: "poll"}},
		End:     capture.End{Known: true, At: 2_000_000},
	}
	for i := 0; i < 400; i++ {
		c.Samples = append(c.Samples, capture.Sample{Stream: 1, At: capture.Ticks(i * 5000),
			Request: readReq(3, 100, 2), Response: regsResp(3, uint16(i+1), 0x5A5A)})
	}
	return collWriteCapture(t, c)
}

// chaosClient misbehaves against addr in one of several ways.
func chaosClient(addr string, rng *rand.Rand) {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	req := func(txn uint16, unit byte, pdu ...byte) []byte {
		b := binary.BigEndian.AppendUint16(nil, txn)
		b = append(b, 0, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(len(pdu)+1))
		return append(append(b, unit), pdu...)
	}
	switch rng.Intn(7) {
	case 0: // connect and leave
	case 1: // garbage
		g := make([]byte, 1+rng.Intn(300))
		rng.Read(g)
		_, _ = conn.Write(g)
	case 2: // half a frame, then silence until the deadline
		_, _ = conn.Write(req(1, 1, 3, 0, 100, 0, 2)[:5])
		_, _ = conn.Read(make([]byte, 64))
	case 3: // a request, then close before the answer
		_, _ = conn.Write(req(2, 1, 3, 0, 100, 0, 2))
	case 4: // many pipelined requests, then a reset
		for i := 0; i < 50; i++ {
			_, _ = conn.Write(req(uint16(i), byte(rng.Intn(3)), 3, 0, byte(rng.Intn(200)), 0, byte(1+rng.Intn(125))))
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetLinger(0)
		}
	case 5: // oversized length field
		b := req(3, 1, 3, 0, 100, 0, 2)
		binary.BigEndian.PutUint16(b[4:], 0xFFFF)
		_, _ = conn.Write(b)
		_, _ = conn.Read(make([]byte, 64))
	case 6: // writes and odd functions the replay refuses, read back
		_, _ = conn.Write(req(4, 1, 6, 0, 100, 0, 9))
		_, _ = conn.Write(req(5, 1, 0x41, 1, 2, 3))
		_, _ = conn.Read(make([]byte, 64))
	}
}

// Misbehaving clients do not disturb the answers to well-behaved ones, the
// server stops promptly, and no goroutine is left behind.
func TestChaosCapture_replayServerUnderAbuse(t *testing.T) {
	path := chaosReplayCapture(t)
	baseline := runtime.NumGoroutine()
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: path, Speed: "1"}, a.Progress(&progress))
	})
	addr := fmt.Sprintf("127.0.0.1:%d", srv.port)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var good, bad atomic.Int64
	for i := 0; i < 4; i++ {
		c, err := gomodbus.New(gomodbus.Config{URL: "tcp://" + addr, Timeout: 2 * time.Second, Logger: gomodbus.NopLogger()})
		require.NoError(t, err)
		require.NoError(t, c.Open())
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { _ = c.Close() }()
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := c.ReadHoldingRegisters(context.Background(), 1, 100, 2)
				if err != nil || got[0] < 1 || got[0] > 400 || got[1] != 0x5A5A {
					bad.Add(1)
					t.Errorf("well-behaved client: %v %v", got, err)
					return
				}
				good.Add(1)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				chaosClient(addr, rng)
			}
		}()
	}
	time.Sleep(1500 * time.Millisecond)
	close(stop)
	wg.Wait()
	require.Zero(t, bad.Load())
	require.Greater(t, good.Load(), int64(100))

	start := time.Now()
	require.ErrorIs(t, srv.stop(t), context.Canceled)
	require.Less(t, time.Since(start), 3*time.Second, "the server stops promptly")
	require.Contains(t, progress.String(), "📊 Served ")
	requireGoroutinesBack(t, baseline)
}

// requireGoroutinesBack waits for the goroutine count to return to about base.
func requireGoroutinesBack(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > base+2 {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			t.Fatalf("goroutines leaked: %d, started with %d\n%s", runtime.NumGoroutine(), base, buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Damaged capture files are rejected with a typed error, or loaded and served,
// but never make anything panic.
func TestChaosCapture_damagedFiles(t *testing.T) {
	dev, _ := e2eCountingDevice()
	rec, _ := e2eRecord(t, "tcp://"+dev.ServeTCP(t), []types.AddressBlock{{StartAddress: 100, RegisterCount: 3}}, 3, 2, 150, false)
	good, err := os.ReadFile(rec)
	require.NoError(t, err)
	var jl bytes.Buffer
	l := capfiletest.Load(t, rec)
	jw, err := jsonl.NewWriter(&jl, l.Header)
	require.NoError(t, err)
	require.NoError(t, l.WriteTo(jw))

	rng := rand.New(rand.NewSource(42))
	dir := t.TempDir()
	for i := 0; i < 400; i++ {
		src := good
		if i%3 == 2 {
			src = jl.Bytes()
		}
		b := append([]byte(nil), src...)
		switch i % 4 {
		case 0:
			b = b[:rng.Intn(len(b))]
		case 1:
			for k := 0; k < 1+rng.Intn(4); k++ {
				b[rng.Intn(len(b))] ^= byte(1 << rng.Intn(8))
			}
		case 2:
			p := rng.Intn(len(b))
			b = append(b[:p], b[p+1+rng.Intn(min(16, len(b)-p)):]...)
		case 3:
			junk := make([]byte, rng.Intn(64))
			rng.Read(junk)
			p := rng.Intn(len(b))
			b = append(append(b[:p:p], junk...), b[p:]...)
		}
		path := filepath.Join(dir, fmt.Sprintf("d%d.mbcap", i))
		require.NoError(t, os.WriteFile(path, b, 0o600))

		l, err := capfile.Load(path)
		if err != nil {
			_, typed := errs.As(capfile.WrapError(err))
			require.True(t, typed, "case %d: %v", i, err)
		} else {
			require.NoError(t, l.Validate(), "case %d: a loaded capture is valid", i)
			for _, tg := range l.Targets {
				d, err := replay.Build(l.Capture, tg.ID, l.End(), replay.Options{})
				require.NoError(t, err)
				_, _ = d.Values(capture.Ticks(rng.Intn(200_000)), 3, 100, 3)
				_, _ = d.Read(0, readReq(3, 99, 5))
			}
		}
		for _, f := range []func() error{
			func() error { return analysis.ExportAddressBlocks(&bytes.Buffer{}, path, config.CaptureSelection{}) },
		} {
			if err := f(); err != nil {
				_, typed := errs.As(capfile.WrapError(err))
				require.True(t, typed, "case %d: %v", i, err)
			}
		}
	}
}

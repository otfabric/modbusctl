package modbus

// Parallel tests of the capture code paths, for the race detector and for
// answers that must not depend on what other goroutines do at the same time.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/stretchr/testify/require"
)

// Many clients read every function of three targets from one static server at
// once; each answer is exactly the recorded one.
func TestParallelCapture_staticServerManyClients(t *testing.T) {
	site := collSite()
	// A third target with a block of 125 registers at 1000.
	site.Targets = append(site.Targets, capture.Target{ID: 3, Unit: 9})
	site.Streams = append(site.Streams, capture.Stream{ID: 4, Target: 3})
	vals := make([]uint16, 125)
	for i := range vals {
		vals[i] = uint16(i*31 + 5)
	}
	site.Samples = append(site.Samples, capture.Sample{Stream: 4, At: 500, Request: readReq(3, 1000, 125), Response: regsResp(3, vals...)})
	path := collWriteCapture(t, site)
	for _, missing := range []string{"exception", "zero"} {
		t.Run(missing, func(t *testing.T) {
			srv, progress := staticServer(t, config.StaticServerConfig{InputFile: path, Missing: missing, MaxClients: 40})
			var wg sync.WaitGroup
			for w := 0; w < 32; w++ {
				c := srv.client(t)
				rng := rand.New(rand.NewSource(int64(w)))
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx := context.Background()
					for i := 0; i < 150; i++ {
						switch rng.Intn(6) {
						case 0:
							got, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
							if err != nil || got[0] != 5 || got[1] != 6 {
								t.Errorf("FC03 unit 21: %v %v", got, err)
							}
						case 1:
							got, err := c.ReadInputRegisters(ctx, 3, 0, 2)
							if err != nil || got[0] != 100 || got[1] != 200 {
								t.Errorf("FC04 unit 3: %v %v", got, err)
							}
						case 2:
							off := uint16(rng.Intn(100))
							qty := uint16(1 + rng.Intn(25))
							got, err := c.ReadHoldingRegisters(ctx, 9, 1000+off, qty)
							if err != nil {
								t.Errorf("FC03 unit 9: %v", err)
								continue
							}
							for k, v := range got {
								if v != vals[int(off)+k] {
									t.Errorf("FC03 unit 9 at %d: %d, want %d", 1000+int(off)+k, v, vals[int(off)+k])
								}
							}
						case 3:
							bits, err := c.ReadCoils(ctx, 3, 0, 4)
							if err != nil || fmt.Sprint(bits) != "[true false true false]" {
								t.Errorf("FC01 unit 3: %v %v", bits, err)
							}
						case 4:
							st, err := c.ReadExceptionStatus(ctx, 21)
							if err != nil || st != 0x6d {
								t.Errorf("FC07: %v %v", st, err)
							}
						case 5:
							got, err := c.ReadHoldingRegisters(ctx, 21, 10, 3) // 12 was never recorded
							if missing == "zero" {
								if err != nil || fmt.Sprint(got) != "[5 6 0]" {
									t.Errorf("zero-filled read: %v %v", got, err)
								}
							} else if err == nil {
								t.Errorf("an unrecorded address was answered: %v", got)
							}
						}
					}
				}()
			}
			wg.Wait()
			require.ErrorIs(t, srv.stop(t), context.Canceled)
			require.Contains(t, progress.String(), "📊 Served 4800 requests: ")
		})
	}
}

// One recorder shared by many goroutines records exactly the attempts that
// succeeded, once each, and survives a Close racing the commits.
func TestParallelCapture_sharedRecorder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.mbcap")
	rec, err := capfile.Create(path, capfile.RecorderOptions{})
	require.NoError(t, err)
	require.NoError(t, rec.AddTarget(capture.Target{ID: 1, Unit: 1}))
	for w := 1; w <= 16; w++ {
		require.NoError(t, rec.AddStream(capture.Stream{ID: uint32(w), Target: 1}))
	}
	var wg sync.WaitGroup
	want := map[string]bool{}
	var mu sync.Mutex
	for w := 1; w <= 16; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				ctx, tok := rec.Begin(context.Background())
				// The address names the goroutine and attempt.
				req := readReq(3, uint16(w*1000+i), 1)
				resp := regsResp(3, uint16(i))
				rec.Tap().OnPDU(ctx, gomodbus.PDUExchange{UnitID: 1, Request: req, Response: resp, Received: time.Now()})
				var cerr error
				if i%4 == 0 {
					cerr = fmt.Errorf("attempt %d failed", i)
				} else {
					mu.Lock()
					want[fmt.Sprintf("%d/%x", w, req)] = true
					mu.Unlock()
				}
				if err := rec.Commit(tok, uint32(w), cerr); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, len(want), rec.Samples())
	require.NoError(t, rec.Close())

	l := capfiletest.Load(t, path)
	got := map[string]bool{}
	for _, s := range l.Samples {
		key := fmt.Sprintf("%d/%x", s.Stream, s.Request)
		require.False(t, got[key], "sample %s twice", key)
		got[key] = true
	}
	require.Equal(t, want, got)

	// Commits racing Close: whatever is committed before Close is in the file, the
	// rest is dropped, and nothing fails or panics.
	path2 := filepath.Join(t.TempDir(), "race.mbcap")
	rec2, err := capfile.Create(path2, capfile.RecorderOptions{})
	require.NoError(t, err)
	require.NoError(t, rec2.AddTarget(capture.Target{ID: 1}))
	require.NoError(t, rec2.AddStream(capture.Stream{ID: 1, Target: 1}))
	start := make(chan struct{})
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 500; i++ {
				ctx, tok := rec2.Begin(context.Background())
				rec2.Tap().OnPDU(ctx, gomodbus.PDUExchange{Request: readReq(4, uint16(w*500+i), 1), Response: regsResp(4, 1), Received: time.Now()})
				if err := rec2.Commit(tok, 1, nil); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	close(start)
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, rec2.Close())
	wg.Wait()
	l2 := capfiletest.Load(t, path2)
	require.False(t, l2.Truncated)
	require.LessOrEqual(t, len(l2.Samples), 4000)
}

// Several recordings run at the same time, each of its own device into its
// own file, without interfering.
func TestParallelCapture_concurrentRecordings(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		dev, _ := e2eCountingDevice()
		url := "tcp://" + dev.ServeTCP(t)
		dir := t.TempDir()
		bf := filepath.Join(dir, "blocks.json")
		require.NoError(t, os.WriteFile(bf, []byte(`[{"start_address":100,"register_count":2},{"start_address":200,"register_count":2}]`), 0o600))
		out := filepath.Join(dir, "rec.mbcap")
		wg.Add(1)
		go func() {
			defer wg.Done()
			sum, err := RecordToCapture(context.Background(), config.RecordConfig{
				DeviceConfig: config.DeviceConfig{URL: url, Unit: 1}, Timeout: coreTimeoutMS, Function: 3,
				Interval: 5, Duration: 300, BlocksFile: bf, OutputFile: out,
			}, nil)
			if err != nil {
				t.Error(err)
				return
			}
			l, err := capfile.Load(out)
			if err != nil {
				t.Error(err)
				return
			}
			if len(l.Samples) != sum.BlocksRecorded || l.Targets[0].Tags["url"] != url {
				t.Errorf("recording %s: %d samples, summary %d", url, len(l.Samples), sum.BlocksRecorded)
			}
			var counter []uint16
			for k := range l.Samples {
				if av, ok := capture.SplitAddresses(&l.Samples[k]); ok && av.Start == 100 {
					counter = append(counter, av.Values[0])
				}
			}
			for k := 1; k < len(counter); k++ {
				if counter[k] != counter[k-1]+1 {
					t.Errorf("recording %s: counter %v: a lost or repeated read", url, counter)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// Concurrent clients of a replay see time move forward together: within one
// loop each client's register 100 never goes back.
func TestParallelCapture_replayClients(t *testing.T) {
	path := chaosReplayCapture(t)
	srv := collStartServer(t, func(ctx context.Context, a collAddr) error {
		return ServeReplay(ctx, config.ReplayServerConfig{Port: a.Port, Listen: a.Listen, InputFile: path, Speed: "2", Loops: 1, MaxClients: 20}, a.Progress(nil))
	})
	var wg sync.WaitGroup
	for w := 0; w < 16; w++ {
		c := srv.client(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			var last uint16
			for i := 0; i < 200; i++ {
				got, err := c.ReadHoldingRegisters(context.Background(), 1, 100, 2)
				if err != nil {
					t.Error(err)
					return
				}
				if got[0] < last || got[1] != 0x5A5A {
					t.Errorf("replayed %v after %d", got, last)
					return
				}
				last = got[0]
			}
		}()
	}
	wg.Wait()
}

// --max-clients bounds the connections a server serves at once; the clients
// within the limit keep working.
func TestParallelCapture_maxClients(t *testing.T) {
	srv, _ := staticServer(t, config.StaticServerConfig{InputFile: collWriteCapture(t, collSite()), MaxClients: 2})
	ctx := context.Background()
	a, b := srv.client(t), srv.client(t)
	for _, c := range []*gomodbus.Client{a, b} {
		_, err := c.ReadHoldingRegisters(ctx, 21, 10, 2)
		require.NoError(t, err)
	}
	third, err := gomodbus.New(gomodbus.Config{URL: fmt.Sprintf("tcp://127.0.0.1:%d", srv.port), Timeout: 500 * time.Millisecond, Logger: gomodbus.NopLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = third.Close() })
	if err := third.Open(); err == nil {
		_, err = third.ReadHoldingRegisters(ctx, 21, 10, 2)
		require.Error(t, err, "a third connection is not served")
	}
	got, err := a.ReadHoldingRegisters(ctx, 21, 10, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{5, 6}, got)
}

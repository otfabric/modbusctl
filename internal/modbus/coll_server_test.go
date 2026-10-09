package modbus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collWriteMCAP writes a capture file into a temp dir and returns its path.
func collWriteMCAP(t *testing.T, fc uint8, records ...types.CaptureRecord) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.mcap")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, mcap.WriteHeader(f, types.CaptureHeader{IP: "10.0.0.1", Port: 502, Unit: 1, Function: fc, StartTime: 1}))
	for _, rec := range records {
		require.NoError(t, mcap.AppendRecord(f, rec))
	}
	require.NoError(t, f.Close())
	return path
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

func collRegRecord(iteration uint32, ts int64, addr uint16, values ...uint16) types.CaptureRecord {
	data := make([]byte, 0, 2*len(values))
	for _, v := range values {
		data = append(data, byte(v>>8), byte(v))
	}
	return types.CaptureRecord{
		Iteration: iteration, RequestTimestamp: ts, ResponseTimestamp: ts + 1,
		StartAddress: addr, RegisterCount: uint16(len(values)), Data: data,
	}
}

func TestCollStaticServer_holdingRegisters(t *testing.T) {
	path := collWriteMCAP(t, 3,
		collRegRecord(1, 100, 10, 1, 2),
		collRegRecord(1, 110, 20, 0x00AA),
		collRegRecord(2, 200, 10, 3, 4),
	)
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: port, Unit: 5, InputFile: path}, &progress)
	})
	c := srv.client(t)
	ctx := context.Background()

	got, err := c.ReadHoldingRegisters(ctx, 5, 10, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{3, 4}, got, "the static server serves the last captured value")

	got, err = c.ReadHoldingRegisters(ctx, 5, 20, 1)
	require.NoError(t, err)
	require.Equal(t, []uint16{0x00AA}, got)

	_, err = c.ReadHoldingRegisters(ctx, 5, 10, 3)
	collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
	_, err = c.ReadHoldingRegisters(ctx, 5, 500, 1)
	collRequireException(t, err, gomodbus.ErrIllegalDataAddress)

	// Only the captured function code is served.
	_, err = c.ReadInputRegisters(ctx, 5, 10, 2)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)
	_, err = c.ReadCoils(ctx, 5, 10, 2)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)
	_, err = c.ReadDiscreteInputs(ctx, 5, 10, 2)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)

	// Other unit IDs are not served.
	_, err = c.ReadHoldingRegisters(ctx, 6, 10, 2)
	require.Error(t, err)

	require.Contains(t, progress.String(), "Static server ready")
	require.Contains(t, progress.String(), "Loaded 3 address entries")
	require.Contains(t, progress.String(), fmt.Sprintf("port %d for unit 5", srv.port))

	require.ErrorIs(t, srv.stop(t), context.Canceled)
	_, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", srv.port), 200*time.Millisecond)
	require.Error(t, err, "the listener must be closed once the context is canceled")
}

func TestCollStaticServer_otherFunctionCodes(t *testing.T) {
	// Ten points at address 0: 1,0,1,0,0,1,0,1 then 0,1.
	bitRecord := types.CaptureRecord{Iteration: 1, RequestTimestamp: 1, ResponseTimestamp: 2, StartAddress: 0, RegisterCount: 10, Data: []byte{0xA5, 0x02}}
	wantBits := []bool{true, false, true, false, false, true, false, true, false, true}
	ctx := context.Background()

	t.Run("coils", func(t *testing.T) {
		path := collWriteMCAP(t, 1, bitRecord)
		srv := collStartServer(t, func(ctx context.Context, port uint16) error {
			// An explicit --function that matches the file is accepted.
			return LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: port, Unit: 1, Function: 1, InputFile: path}, nil)
		})
		c := srv.client(t)
		// The whole block in one request, and every point on its own.
		all, err := c.ReadCoils(ctx, 1, 0, 10)
		require.NoError(t, err)
		require.Equal(t, wantBits, all)
		for addr, want := range wantBits {
			got, err := c.ReadCoils(ctx, 1, uint16(addr), 1)
			require.NoError(t, err)
			require.Equalf(t, []bool{want}, got, "coil %d", addr)
		}
		_, err = c.ReadCoils(ctx, 1, 5, 10)
		collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = c.ReadCoils(ctx, 2, 0, 1)
		require.Error(t, err, "other unit IDs are not served")
		_, err = c.ReadDiscreteInputs(ctx, 1, 0, 10)
		collRequireException(t, err, gomodbus.ErrIllegalFunction)
		_, err = c.ReadHoldingRegisters(ctx, 1, 0, 1)
		collRequireException(t, err, gomodbus.ErrIllegalFunction)
	})

	t.Run("discrete inputs", func(t *testing.T) {
		path := collWriteMCAP(t, 2, bitRecord)
		srv := collStartServer(t, func(ctx context.Context, port uint16) error {
			return LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: port, Unit: 1, InputFile: path}, nil)
		})
		c := srv.client(t)
		all, err := c.ReadDiscreteInputs(ctx, 1, 2, 8)
		require.NoError(t, err)
		require.Equal(t, wantBits[2:], all)
		for addr, want := range wantBits {
			got, err := c.ReadDiscreteInputs(ctx, 1, uint16(addr), 1)
			require.NoError(t, err)
			require.Equalf(t, []bool{want}, got, "discrete input %d", addr)
		}
		_, err = c.ReadDiscreteInputs(ctx, 1, 9, 2)
		collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = c.ReadDiscreteInputs(ctx, 2, 0, 1)
		require.Error(t, err, "other unit IDs are not served")
		_, err = c.ReadCoils(ctx, 1, 0, 10)
		collRequireException(t, err, gomodbus.ErrIllegalFunction)
	})

	t.Run("input registers", func(t *testing.T) {
		path := collWriteMCAP(t, 4, collRegRecord(1, 1, 7, 0xCAFE, 0xF00D))
		srv := collStartServer(t, func(ctx context.Context, port uint16) error {
			return LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: port, Unit: 1, InputFile: path}, nil)
		})
		c := srv.client(t)
		got, err := c.ReadInputRegisters(ctx, 1, 7, 2)
		require.NoError(t, err)
		require.Equal(t, []uint16{0xCAFE, 0xF00D}, got)
		_, err = c.ReadInputRegisters(ctx, 1, 8, 2)
		collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = c.ReadInputRegisters(ctx, 2, 7, 1)
		require.Error(t, err, "other unit IDs are not served")
		_, err = c.ReadHoldingRegisters(ctx, 1, 7, 2)
		collRequireException(t, err, gomodbus.ErrIllegalFunction)
	})
}

func TestCollStaticServer_errors(t *testing.T) {
	good := collWriteMCAP(t, 3, collRegRecord(1, 1, 0, 1))

	t.Run("context already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: 1, InputFile: good}, io.Discard)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("missing file", func(t *testing.T) {
		err := LoadMCAPAndServeStatic(context.Background(), config.StaticServerConfig{InputFile: filepath.Join(t.TempDir(), "nope.mcap")}, nil)
		collRequireErr(t, err, errs.KindOutput, errs.CodeMCAPLoadFailed)
	})
	t.Run("not an mcap file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "junk.mcap")
		require.NoError(t, os.WriteFile(path, []byte("not a capture"), 0o600))
		err := LoadMCAPAndServeStatic(context.Background(), config.StaticServerConfig{InputFile: path}, nil)
		collRequireErr(t, err, errs.KindOutput, errs.CodeMCAPLoadFailed)
	})
	t.Run("function mismatch", func(t *testing.T) {
		err := LoadMCAPAndServeStatic(context.Background(), config.StaticServerConfig{Function: 4, InputFile: good}, nil)
		e := collTypedErr(t, err, errs.KindInvalidInput, errs.CodeInvalidFlagCombination)
		require.Contains(t, e.Message, "FC=3")
		require.Contains(t, e.Message, "--function=4")
	})
	t.Run("record data does not match its register count", func(t *testing.T) {
		bad := collWriteMCAP(t, 3, types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 2, Data: []byte{1, 2, 3}})
		err := LoadMCAPAndServeStatic(context.Background(), config.StaticServerConfig{InputFile: bad}, nil)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})
	t.Run("coil record shorter than its point count", func(t *testing.T) {
		bad := collWriteMCAP(t, 1, types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 9, Data: []byte{0xFF}})
		err := LoadMCAPAndServeStatic(context.Background(), config.StaticServerConfig{InputFile: bad}, nil)
		collRequireErr(t, err, errs.KindInvalidInput, errs.CodeInvalidInput)
	})
	t.Run("port already in use", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		defer func() { _ = ln.Close() }()
		port := uint16(ln.Addr().(*net.TCPAddr).Port)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = LoadMCAPAndServeStatic(ctx, config.StaticServerConfig{Port: port, InputFile: good, Debug: true}, nil)
		collRequireErr(t, err, errs.KindTransport, errs.CodeTransportConnectFailed)
		require.NoError(t, ctx.Err(), "a bind failure must be reported immediately")
	})
}

func TestCollMemoryStore(t *testing.T) {
	t.Run("last values win and data is copied", func(t *testing.T) {
		data := []byte{0x00, 0x01}
		s, err := newMemoryStoreLastValues([]types.CaptureRecord{
			{Iteration: 1, StartAddress: 4, RegisterCount: 1, Data: data},
			{Iteration: 2, StartAddress: 4, RegisterCount: 1, Data: []byte{0x00, 0x02}},
			{Iteration: 1, StartAddress: 5, RegisterCount: 1, Data: data},
		}, 3)
		require.NoError(t, err)
		data[1] = 0xFF
		got, err := s.get(3, 4, 2)
		require.NoError(t, err)
		require.Equal(t, []byte{0x00, 0x02, 0x00, 0x01}, got)
		got, err = s.get(4, 5, 1)
		require.NoError(t, err)
		require.Equal(t, []byte{0x00, 0x01}, got, "FC03 and FC04 share the register map")
	})

	t.Run("get errors", func(t *testing.T) {
		s, err := newMemoryStoreLastValues(nil, 3)
		require.NoError(t, err)
		_, err = s.get(3, 0, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = s.get(1, 0, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = s.get(2, 0, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
		_, err = s.get(5, 0, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalFunction)
		got, err := s.get(3, 0, 0)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("invalid record", func(t *testing.T) {
		_, err := newMemoryStoreLastValues([]types.CaptureRecord{{StartAddress: 3, RegisterCount: 1, Data: []byte{1}}}, 3)
		require.Error(t, err)
		_, err = newMemoryStoreLastValues([]types.CaptureRecord{{StartAddress: 3, RegisterCount: 0}}, 1)
		require.Error(t, err, "a coil record without points is invalid")
	})

	t.Run("first values keep the first iteration only", func(t *testing.T) {
		var warn collSyncBuffer
		s := newMemoryStoreFirstValues([]types.CaptureRecord{
			collRegRecord(7, 1, 0, 11),
			{Iteration: 7, StartAddress: 9, RegisterCount: 2, Data: []byte{1}},
			collRegRecord(8, 2, 0, 22),
			collRegRecord(8, 2, 1, 33),
		}, 3, &warn)
		got, err := s.get(3, 0, 1)
		require.NoError(t, err)
		require.Equal(t, []byte{0, 11}, got)
		_, err = s.get(3, 1, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress, "later iterations must not be loaded up front")
		require.Contains(t, warn.String(), "Failed to write record at address 9")

		require.NoError(t, s.upsertRecord(collRegRecord(8, 2, 1, 33), 3))
		got, err = s.get(3, 0, 2)
		require.NoError(t, err)
		require.Equal(t, []byte{0, 11, 0, 33}, got)
		require.Error(t, s.upsertRecord(types.CaptureRecord{StartAddress: 1, RegisterCount: 1}, 3))
	})

	t.Run("first values of an empty capture", func(t *testing.T) {
		s := newMemoryStoreFirstValues(nil, 3, nil)
		_, err := s.get(3, 0, 1)
		require.ErrorIs(t, err, gomodbus.ErrIllegalDataAddress)
	})
}

// collFailingStore fails every lookup with a fixed error.
type collFailingStore struct{ err error }

func (s collFailingStore) get(uint8, uint16, uint16) ([]byte, error) { return nil, s.err }

func TestCollStaticHandler_storeErrors(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		storeErr error
		want     error
	}{
		{name: "illegal function is passed through", storeErr: gomodbus.ErrIllegalFunction, want: gomodbus.ErrIllegalFunction},
		{name: "anything else becomes illegal data address", storeErr: errors.New("boom"), want: gomodbus.ErrIllegalDataAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := func(fc uint8) *StaticHandler {
				return &StaticHandler{Store: collFailingStore{err: tc.storeErr}, Cfg: ServerConfig{Unit: 1, Function: fc}}
			}
			_, err := handler(1).HandleCoils(ctx, &gomodbus.CoilsRequest{UnitID: 1, Quantity: 1})
			assert.ErrorIs(t, err, tc.want)
			_, err = handler(2).HandleDiscreteInputs(ctx, &gomodbus.DiscreteInputsRequest{UnitID: 1, Quantity: 1})
			assert.ErrorIs(t, err, tc.want)
			_, err = handler(3).HandleHoldingRegisters(ctx, &gomodbus.HoldingRegistersRequest{UnitID: 1, Quantity: 1})
			assert.ErrorIs(t, err, tc.want)
			_, err = handler(4).HandleInputRegisters(ctx, &gomodbus.InputRegistersRequest{UnitID: 1, Quantity: 1})
			assert.ErrorIs(t, err, tc.want)
		})
	}

	t.Run("wrong unit", func(t *testing.T) {
		store, err := newMemoryStoreLastValues(nil, 3)
		require.NoError(t, err)
		for fc := uint8(1); fc <= 4; fc++ {
			h := &StaticHandler{Store: store, Cfg: ServerConfig{Unit: 1, Function: fc}}
			switch fc {
			case 1:
				_, err = h.HandleCoils(ctx, &gomodbus.CoilsRequest{UnitID: 2, Quantity: 1})
			case 2:
				_, err = h.HandleDiscreteInputs(ctx, &gomodbus.DiscreteInputsRequest{UnitID: 2, Quantity: 1})
			case 3:
				_, err = h.HandleHoldingRegisters(ctx, &gomodbus.HoldingRegistersRequest{UnitID: 2, Quantity: 1})
			default:
				_, err = h.HandleInputRegisters(ctx, &gomodbus.InputRegistersRequest{UnitID: 2, Quantity: 1})
			}
			assert.ErrorIsf(t, err, gomodbus.ErrBadUnitID, "FC%d", fc)
		}
	})
}

func TestCollReplayServer_holdsFirstIterationUntilItsTimeComes(t *testing.T) {
	// Ten minutes of capture time separate the iterations: with interval 0 the replay keeps that spacing.
	gap := int64(10 * time.Minute)
	path := collWriteMCAP(t, 3,
		collRegRecord(1, 1000, 0, 100, 101),
		collRegRecord(2, 1000+gap, 0, 200, 201),
	)
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: port, Unit: 2, Loops: 1, InputFile: path}, &progress)
	})
	c := srv.client(t)
	ctx := context.Background()

	require.Eventually(t, func() bool {
		return len(progress.String()) > 0 && collContainsAll(progress.String(), "Processing iteration 2")
	}, 2*time.Second, 5*time.Millisecond)
	got, err := c.ReadHoldingRegisters(ctx, 2, 0, 2)
	require.NoError(t, err)
	require.Equal(t, []uint16{100, 101}, got)

	_, err = c.ReadInputRegisters(ctx, 2, 0, 2)
	collRequireException(t, err, gomodbus.ErrIllegalFunction)
	_, err = c.ReadHoldingRegisters(ctx, 3, 0, 2)
	require.Error(t, err, "other unit IDs are not served")

	out := progress.String()
	require.Contains(t, out, "Replay server ready")
	require.Contains(t, out, "Loaded 2 address entries")
	require.Contains(t, out, "Starting replay loop 1/1")
	require.Contains(t, out, "Processing iteration 1 with 1 records")

	// Cancel interrupts the pending wait and stops the server.
	require.ErrorIs(t, srv.stop(t), context.Canceled)
}

func collContainsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestCollReplayServer_advancesThroughIterations(t *testing.T) {
	path := collWriteMCAP(t, 4,
		collRegRecord(1, 1000, 0, 1),
		collRegRecord(2, 2000, 0, 2),
		collRegRecord(2, 2001, 1, 22),
		collRegRecord(3, 3000, 0, 3),
	)
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		// Function matches the file; a fixed 1 ms interval replaces the captured spacing.
		return LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: port, Unit: 1, Function: 4, Loops: 2, Interval: 1, InputFile: path}, &progress)
	})
	c := srv.client(t)

	require.Eventually(t, func() bool {
		return collContainsAll(progress.String(), "Starting replay loop 2/2")
	}, 3*time.Second, 5*time.Millisecond, "the replay must loop twice")

	// After the last loop the store holds the last iteration, plus everything seen before it.
	require.Eventually(t, func() bool {
		got, err := c.ReadInputRegisters(context.Background(), 1, 0, 2)
		return err == nil && len(got) == 2 && got[0] == 3 && got[1] == 22
	}, 3*time.Second, 5*time.Millisecond)

	require.NotContains(t, progress.String(), "Starting replay loop 3/2")
	require.ErrorIs(t, srv.stop(t), context.Canceled)
}

func TestCollReplayServer_reportsBadRecords(t *testing.T) {
	path := collWriteMCAP(t, 3,
		collRegRecord(1, 1000, 0, 5),
		types.CaptureRecord{Iteration: 1, RequestTimestamp: 1001, StartAddress: 40, RegisterCount: 2, Data: []byte{9}},
	)
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: port, Unit: 1, Loops: 1, InputFile: path}, &progress)
	})
	c := srv.client(t)

	got, err := c.ReadHoldingRegisters(context.Background(), 1, 0, 1)
	require.NoError(t, err)
	require.Equal(t, []uint16{5}, got)
	_, err = c.ReadHoldingRegisters(context.Background(), 1, 40, 1)
	collRequireException(t, err, gomodbus.ErrIllegalDataAddress)
	require.Contains(t, progress.String(), "Failed to write record at address 40")
}

func TestCollReplayServer_errors(t *testing.T) {
	good := collWriteMCAP(t, 3, collRegRecord(1, 1, 0, 1))

	t.Run("context already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: 1, InputFile: good}, io.Discard)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("missing file", func(t *testing.T) {
		err := LoadMCAPAndServeReplay(context.Background(), config.ReplayServerConfig{InputFile: filepath.Join(t.TempDir(), "nope.mcap")}, nil)
		collRequireErr(t, err, errs.KindOutput, errs.CodeMCAPLoadFailed)
	})
	t.Run("function mismatch", func(t *testing.T) {
		err := LoadMCAPAndServeReplay(context.Background(), config.ReplayServerConfig{Function: 1, InputFile: good}, nil)
		e := collTypedErr(t, err, errs.KindInvalidInput, errs.CodeInvalidFlagCombination)
		require.Contains(t, e.Message, "FC=3")
		require.Contains(t, e.Message, "--function=1")
	})
	t.Run("port already in use", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		defer func() { _ = ln.Close() }()
		port := uint16(ln.Addr().(*net.TCPAddr).Port)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: port, Loops: 1, InputFile: good}, nil)
		collRequireErr(t, err, errs.KindTransport, errs.CodeTransportConnectFailed)
		require.NoError(t, ctx.Err(), "a bind failure must be reported immediately")
	})
}

// With --loops 0 the replay repeats until it is canceled.
func TestCollReplayServer_loopsForeverUntilCanceled(t *testing.T) {
	path := collWriteMCAP(t, 3, collRegRecord(1, 1000, 0, 1), collRegRecord(2, 2000, 0, 2))
	var progress collSyncBuffer
	srv := collStartServer(t, func(ctx context.Context, port uint16) error {
		return LoadMCAPAndServeReplay(ctx, config.ReplayServerConfig{Port: port, Unit: 1, Interval: 1, InputFile: path}, &progress)
	})
	require.Eventually(t, func() bool {
		return collContainsAll(progress.String(), "Starting replay loop 3/0")
	}, 3*time.Second, 2*time.Millisecond)
	require.ErrorIs(t, srv.stop(t), context.Canceled)

	// The replay goroutine must stop too: its output settles.
	require.Eventually(t, func() bool {
		before := progress.String()
		time.Sleep(20 * time.Millisecond)
		return before == progress.String()
	}, 2*time.Second, time.Millisecond)
}

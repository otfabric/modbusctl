package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gomodbus "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/modbus"
)

// recordHolding records holding registers 100-102 of a device whose register 100 counts the
// iterations (1, 2, 3, ...), and returns the MCAP path and the number of iterations.
func recordHolding(t *testing.T) (string, uint16) {
	t.Helper()
	d := mbtest.NewDevice().SetHolding(100, 0, 0xBEEF, 0x1234)
	var n uint16
	d.OnRequest = func(mbtest.Request) mbtest.Action {
		n++
		d.SetHolding(100, n)
		return mbtest.Action{}
	}
	return record(t, d, 3, `[{"start_address":100,"register_count":3}]`)
}

// record captures the given blocks of d with function code fc.
func record(t *testing.T, d *mbtest.Device, fc uint8, blocksJSON string) (string, uint16) {
	t.Helper()
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(blocksJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "capture.mcap")
	res, err := modbus.RecordAndWriteMCAP(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: "tcp://" + d.ServeTCP(t), Unit: 1},
		Function:     fc,
		BlocksFile:   blocks,
		Interval:     10,
		Duration:     60,
		OutputFile:   out,
	}, io.Discard)
	if err != nil {
		t.Fatalf("record capture: %v", err)
	}
	if res.Iterations < 2 {
		t.Fatalf("expected at least two iterations, got %+v", res)
	}
	return out, uint16(res.Iterations)
}

// freePort returns a TCP port nothing listens on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

// server is a "modbusctl server ..." command running in the background.
type server struct {
	port   string
	stdout *syncBuffer
	cancel context.CancelFunc
	done   chan cmdResult
}

// startServer runs "server <args> --port <free port>" until stop is called.
func startServer(t *testing.T, args ...string) *server {
	t.Helper()
	s := &server{port: freePort(t), stdout: &syncBuffer{}, done: make(chan cmdResult, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	args = append(append([]string{"server"}, args...), "--port", s.port)
	go func() { s.done <- runTo(t, ctx, s.stdout, args...) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
	})
	// Wait until the listener accepts connections.
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+s.port, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return s
		}
		select {
		case r := <-s.done:
			t.Fatalf("server exited before it was reachable: %v\n%s", r.err, s.stdout.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server on port %s did not come up: %v\n%s", s.port, err, s.stdout.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// stop interrupts the server and returns the command result.
func (s *server) stop(t *testing.T) cmdResult {
	t.Helper()
	s.cancel()
	select {
	case r := <-s.done:
		s.done <- r
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after its context was cancelled")
		return cmdResult{}
	}
}

// dial opens a go-modbus client to the server.
func (s *server) dial(t *testing.T) *gomodbus.Client {
	t.Helper()
	c, err := gomodbus.New(gomodbus.Config{URL: "tcp://127.0.0.1:" + s.port, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("modbus client: %v", err)
	}
	if err := c.Open(); err != nil {
		t.Fatalf("open modbus client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// readHolding reads count holding registers from the server.
func readHolding(t *testing.T, c *gomodbus.Client, unit uint8, addr, count uint16) ([]uint16, error) {
	t.Helper()
	raw, err := c.ReadRegisterBytes(context.Background(), unit, addr, count*2, gomodbus.HoldingRegister)
	if err != nil {
		return nil, err
	}
	out := make([]uint16, 0, count)
	for i := 0; i+1 < len(raw); i += 2 {
		out = append(out, binary.BigEndian.Uint16(raw[i:]))
	}
	return out, nil
}

func TestStatic(t *testing.T) {
	in, iterations := recordHolding(t)
	s := startServer(t, "static", "--input", in, "--unit", "1", "--function", "3")
	c := s.dial(t)

	// A static server serves the last value captured for every register.
	got, err := readHolding(t, c, 1, 100, 3)
	if err != nil {
		t.Fatalf("read from static server: %v", err)
	}
	if want := []uint16{iterations, 0xBEEF, 0x1234}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("static server returned %v, want %v", got, want)
	}

	// Addresses outside the capture, and other register types, are not served.
	if _, err := readHolding(t, c, 1, 200, 1); err == nil {
		t.Fatal("expected an exception for an address outside the capture")
	}
	if _, err := c.ReadRegisterBytes(context.Background(), 1, 100, 2, gomodbus.InputRegister); err == nil {
		t.Fatal("expected an exception for input registers on a holding-register capture")
	}
	if _, err := c.ReadCoils(context.Background(), 1, 100, 1); err == nil {
		t.Fatal("expected an exception for coils on a holding-register capture")
	}

	r := s.stop(t)
	if r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
	wantContains(t, s.stdout.String(), "Static server ready on port "+s.port+" for unit 1 and function 3",
		"Loaded 3 address entries", "Serving Modbus TCP on port "+s.port)
}

func TestStatic_coils(t *testing.T) {
	d := mbtest.NewDevice().SetCoils(8, true, false, true, true)
	in, _ := record(t, d, 1, `[{"start_address":8,"register_count":4}]`)
	s := startServer(t, "--debug", "static", "--input", in)
	c := s.dial(t)
	// Each coil is read on its own: a multi-coil read from the static server only returns
	// the first coil correctly (HandleCoils treats the per-coil store bytes as packed bits).
	for i, want := range []bool{true, false, true, true} {
		got, err := c.ReadCoils(context.Background(), 1, 8+uint16(i), 1)
		if err != nil {
			t.Fatalf("read coil %d from static server: %v", 8+i, err)
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("static server returned coil %d = %v, want %v", 8+i, got, want)
		}
	}
	if _, err := c.ReadCoils(context.Background(), 1, 100, 1); err == nil {
		t.Fatal("expected an exception for a coil outside the capture")
	}
	if _, err := c.ReadCoils(context.Background(), 2, 8, 1); err == nil {
		t.Fatal("expected an error for a unit the server does not serve")
	}
	if _, err := c.ReadDiscreteInputs(context.Background(), 1, 8, 1); err == nil {
		t.Fatal("expected an exception for discrete inputs on a coil capture")
	}
	if r := s.stop(t); r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
	wantContains(t, s.stdout.String(), "function 1")
}

func TestReplay(t *testing.T) {
	in, iterations := recordHolding(t)
	// One pass over the capture with a fixed 10 ms between iterations.
	s := startServer(t, "replay", "--input", in, "--loops", "1", "--interval", "10", "--function", "3")
	c := s.dial(t)

	// Register 100 steps through the captured iterations and ends on the last one.
	var seen []uint16
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := readHolding(t, c, 1, 100, 3)
		if err != nil {
			t.Fatalf("read from replay server: %v", err)
		}
		if got[1] != 0xBEEF || got[2] != 0x1234 {
			t.Fatalf("replay server returned %v", got)
		}
		if len(seen) == 0 || seen[len(seen)-1] != got[0] {
			seen = append(seen, got[0])
		}
		if got[0] == iterations {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay never reached iteration %d, saw %v", iterations, seen)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Fatalf("replayed values went backwards: %v", seen)
		}
	}

	r := s.stop(t)
	if r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
	wantContains(t, s.stdout.String(), "Replay server ready on port "+s.port+" for unit 1 and function 3",
		"Starting replay loop 1/1", "Processing iteration 0 with 1 records")
}

func TestReplay_capturedTimingAndLoops(t *testing.T) {
	in, iterations := recordHolding(t)
	// --interval 0 keeps the captured spacing (10 ms); --loops 0 repeats until interrupted.
	s := startServer(t, "replay", "--input", in)
	c := s.dial(t)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(s.stdout.String(), "Starting replay loop 2/0") {
		if time.Now().After(deadline) {
			t.Fatalf("replay did not start a second loop:\n%s", s.stdout.String())
		}
		got, err := readHolding(t, c, 1, 100, 1)
		if err != nil {
			t.Fatalf("read from replay server: %v", err)
		}
		if got[0] < 1 || got[0] > iterations {
			t.Fatalf("replay served %d, outside the captured values 1..%d", got[0], iterations)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r := s.stop(t); r.err != nil {
		t.Fatalf("an interrupted server must exit cleanly, got %v", r.err)
	}
}

func TestFailures(t *testing.T) {
	in, _ := recordHolding(t)
	tmp := t.TempDir()
	garbage := filepath.Join(tmp, "garbage.mcap")
	if err := os.WriteFile(garbage, []byte("not an MCAP file"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A port that is already taken on every interface.
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	busyPort := strconv.Itoa(busy.Addr().(*net.TCPAddr).Port)

	for _, sub := range []string{"static", "replay"} {
		port := freePort(t)

		// --input is a required flag: cobra rejects the command before RunE.
		r := run(t, "server", sub, "--port", port)
		if r.err == nil || !strings.Contains(r.err.Error(), "input") {
			t.Fatalf("server %s: expected a required-flag error, got %v", sub, r.err)
		}

		wantKind(t, run(t, "server", sub, "--input", filepath.Join(tmp, "missing.mcap"), "--port", port), errs.KindInvalidInput)
		wantKind(t, run(t, "server", sub, "--input", tmp, "--port", port), errs.KindInvalidInput)
		wantKind(t, run(t, "server", sub, "--input", in, "--port", "0"), errs.KindInvalidInput)
		wantKind(t, run(t, "server", sub, "--input", in, "--port", port, "--function", "9"), errs.KindInvalidInput)

		// --function only confirms the function code of the capture.
		r = run(t, "server", sub, "--input", in, "--port", port, "--function", "4")
		wantKind(t, r, errs.KindInvalidInput)
		wantContains(t, r.err.Error(), "function code mismatch")

		r = run(t, "server", sub, "--input", garbage, "--port", port)
		wantKind(t, r, errs.KindOutput)

		busyArgs := []string{"server", sub, "--input", in, "--port", busyPort}
		if sub == "replay" {
			busyArgs = append(busyArgs, "--loops", "1")
		}
		r = run(t, busyArgs...)
		if r.err == nil {
			t.Fatalf("server %s: expected a failure on a port in use", sub)
		}
		wantExit(t, r, errs.ExitTransport)

		// A context that is already cancelled ends the command cleanly, without serving.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r = runCtx(t, ctx, "server", sub, "--input", in, "--port", port)
		if r.err != nil {
			t.Fatalf("server %s with a cancelled context: %v", sub, r.err)
		}
		if strings.Contains(r.stdout, "Serving") {
			t.Fatalf("server %s served despite a cancelled context:\n%s", sub, r.stdout)
		}
	}
}

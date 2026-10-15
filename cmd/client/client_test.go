package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
)

// newDevice returns a device with a few coils and registers and its tcp:// URL.
func newDevice(t *testing.T) (*mbtest.Device, string) {
	t.Helper()
	d := mbtest.NewDevice().
		SetHolding(0, 0).
		SetHolding(100, 1, 2, 3, 0x41AC, 0x0000).
		SetHolding(200, 0, 0).
		SetInput(0, 7, 8).
		SetCoils(8, true, false, true).
		SetCoils(20, false).
		SetCoils(30, false, false, false, false).
		SetDiscreteInputs(0, true, true)
	return d, "tcp://" + d.ServeTCP(t)
}

func TestRead_formats(t *testing.T) {
	_, url := newDevice(t)
	tmp := t.TempDir()

	out := filepath.Join(tmp, "text.mbcap")
	r := run(t, "client", "read", "--url", url, "--start", "100", "--count", "3", "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Reading 3 registers starting from address 100 using function code 3", "00 01 00 02 00 03", out)
	wantFile(t, out)

	out = filepath.Join(tmp, "json.mbcap")
	r = run(t, "client", "read", "--url", url, "--start", "100", "--count", "3", "--output", out, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.ReadResult
	decodeJSON(t, r.stdout, &res)
	want := types.ReadResult{Target: url, UnitID: 1, Function: 3, StartAddress: 100, RegisterCount: 3, RawByteCount: 6, RawDataHex: "00 01 00 02 00 03", CaptureOutputPath: out}
	if res != want {
		t.Fatalf("read result %+v, want %+v", res, want)
	}
	wantFile(t, out)

	out = filepath.Join(tmp, "table.mbcap")
	r = run(t, "client", "read", "--url", url, "--start", "100", "--count", "3", "--output", out, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "raw_data_hex", "00 01 00 02 00 03", "register_count")
	wantFile(t, out)
}

func TestRead_functionsAndEndpoints(t *testing.T) {
	_, url := newDevice(t)
	host, port, err := net.SplitHostPort(strings.TrimPrefix(url, "tcp://"))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()

	// Input registers through --ip/--port instead of --url.
	r := run(t, "client", "read", "--ip", host, "--port", port, "--function", "4", "--start", "0", "--count", "2",
		"--output", filepath.Join(tmp, "in.mbcap"), "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.ReadResult
	decodeJSON(t, r.stdout, &res)
	if res.Function != 4 || res.RawDataHex != "00 07 00 08" {
		t.Fatalf("unexpected input register read: %+v", res)
	}

	// Coils: 1,0,1 packs into 0x05.
	r = run(t, "client", "read", "--url", url, "--function", "1", "--start", "8", "--count", "3",
		"--output", filepath.Join(tmp, "coils.mbcap"), "--format", "json")
	wantExit(t, r, errs.ExitOK)
	decodeJSON(t, r.stdout, &res)
	if res.Function != 1 || res.RawDataHex != "05" {
		t.Fatalf("unexpected coil read: %+v", res)
	}

	// Discrete inputs, with root --debug enabled.
	r = run(t, "--debug", "client", "read", "--url", url, "--function", "2", "--start", "0", "--count", "2",
		"--output", filepath.Join(tmp, "di.mbcap"), "--format", "json", "--timeout", "500")
	wantExit(t, r, errs.ExitOK)
	decodeJSON(t, r.stdout, &res)
	if res.Function != 2 || res.RawDataHex != "03" {
		t.Fatalf("unexpected discrete input read: %+v", res)
	}
}

func TestRead_failures(t *testing.T) {
	d, url := newDevice(t)
	out := filepath.Join(t.TempDir(), "x.mbcap")

	wantKind(t, run(t, "client", "read", "--url", url, "--start", "100", "--format", "xml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--start", "100", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--ip", "not-an-ip", "--start", "100", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--url", url, "--ip", "127.0.0.1", "--start", "100", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--url", url, "--function", "9", "--start", "100", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--url", url, "--start", "100", "--count", "0", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--url", url, "--start", "100", "--timeout", "50", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "read", "--url", "http://127.0.0.1:502", "--start", "100", "--output", out), errs.KindInvalidInput)
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("validation failures must not reach the device, saw %d requests", n)
	}

	r := run(t, "client", "read", "--url", refusedURL(t), "--start", "100", "--output", out)
	wantKind(t, r, errs.KindTransport)
	wantExit(t, r, errs.ExitTransport)

	// An address the device does not have is a Modbus exception.
	r = run(t, "client", "read", "--url", url, "--start", "9000", "--output", out)
	wantKind(t, r, errs.KindModbus)
	wantExit(t, r, errs.ExitProtocol)

	// An unknown flag is a plain cobra error (internal exit code).
	r = run(t, "client", "read", "--no-such-flag")
	if r.err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
	wantExit(t, r, errs.ExitInternal)
}

func TestRead_timeout(t *testing.T) {
	d, url := newDevice(t)
	d.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Drop: true} }
	r := run(t, "client", "read", "--url", url, "--start", "100", "--timeout", "200", "--output", filepath.Join(t.TempDir(), "x.mbcap"))
	if r.err == nil {
		t.Fatalf("expected a timeout, got success:\n%s", r.stdout)
	}
	if code := r.exit(); code != errs.ExitTimeout && code != errs.ExitTransport {
		t.Fatalf("exit code %d, want timeout (5) or transport (4): %v", code, r.err)
	}
}

func TestDecode(t *testing.T) {
	_, url := newDevice(t)

	r := run(t, "client", "decode", "--url", url, "--start", "103", "--codec", "float32/layout:4321", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.DecodeResult
	decodeJSON(t, r.stdout, &res)
	if res.Codec != "float32/layout:4321" || res.Family != "float" || res.ValueKind != "float32" ||
		res.RegisterCount != 2 || res.RawDataHex != "41 AC 00 00" || res.StartAddress != 103 {
		t.Fatalf("unexpected decode result: %+v", res)
	}
	if len(res.Values) != 1 || res.Values[0] != 21.5 {
		t.Fatalf("decoded values %v, want [21.5]", res.Values)
	}

	r = run(t, "client", "decode", "--url", url, "--start", "103", "--codec", "float32/layout:4321")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "21.5", "float32/layout:4321")

	r = run(t, "client", "decode", "--url", url, "--start", "103", "--codec", "float32/layout:4321", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "21.5")

	// Input registers decode too.
	r = run(t, "client", "decode", "--url", url, "--function", "4", "--start", "0", "--codec", "float32/layout:4321", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	decodeJSON(t, r.stdout, &res)
	if res.Function != 4 || res.RawDataHex != "00 07 00 08" {
		t.Fatalf("unexpected input decode: %+v", res)
	}

	wantKind(t, run(t, "client", "decode", "--url", url, "--start", "103", "--codec", "nope"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "decode", "--url", url, "--start", "103"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "decode", "--url", url, "--start", "8", "--function", "1", "--codec", "float32/layout:4321"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "decode", "--url", url, "--start", "103", "--codec", "float32/layout:4321", "--format", "yaml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "decode", "--url", refusedURL(t), "--start", "103", "--codec", "float32/layout:4321"), errs.KindTransport)
	wantKind(t, run(t, "client", "decode", "--url", url, "--start", "9000", "--codec", "float32/layout:4321"), errs.KindModbus)
}

func TestCodecs(t *testing.T) {
	r := run(t, "client", "codecs", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var all types.CodecList
	decodeJSON(t, r.stdout, &all)
	if len(all.Codecs) < 10 {
		t.Fatalf("expected the full codec list, got %d entries", len(all.Codecs))
	}
	for i := 1; i < len(all.Codecs); i++ {
		if all.Codecs[i-1].ID >= all.Codecs[i].ID {
			t.Fatalf("codecs are not sorted by ID: %q before %q", all.Codecs[i-1].ID, all.Codecs[i].ID)
		}
	}

	r = run(t, "client", "codecs", "--family", " Float ", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var floats types.CodecList
	decodeJSON(t, r.stdout, &floats)
	if len(floats.Codecs) == 0 || len(floats.Codecs) >= len(all.Codecs) {
		t.Fatalf("family filter returned %d of %d codecs", len(floats.Codecs), len(all.Codecs))
	}
	for _, c := range floats.Codecs {
		if c.Family != "float" {
			t.Fatalf("family filter let %+v through", c)
		}
	}

	r = run(t, "client", "codecs", "--value-kind", "float32", "--registers", "2", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var f32 types.CodecList
	decodeJSON(t, r.stdout, &f32)
	if len(f32.Codecs) == 0 {
		t.Fatal("no float32 codecs found")
	}
	for _, c := range f32.Codecs {
		if c.ValueKind != "float32" || c.Registers != 2 || c.Bytes != 4 {
			t.Fatalf("value-kind/registers filter let %+v through", c)
		}
	}

	// Filters that exclude each other give an empty list, not an error.
	r = run(t, "client", "codecs", "--value-kind", "float32", "--registers", "4", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var none types.CodecList
	decodeJSON(t, r.stdout, &none)
	if len(none.Codecs) != 0 {
		t.Fatalf("expected no codecs, got %+v", none.Codecs)
	}

	r = run(t, "client", "codecs", "--family", "float", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "value_kind", "float32/layout:4321", "float64/layout:12345678")
	if strings.Contains(r.stdout, "uint") {
		t.Fatalf("table lists non-float codecs:\n%s", r.stdout)
	}

	r = run(t, "client", "codecs", "--family", "float")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "float32/layout:4321")

	wantKind(t, run(t, "client", "codecs", "--format", "xml"), errs.KindInvalidInput)
}

func TestWrite_registers(t *testing.T) {
	d, url := newDevice(t)

	// One value: FC6.
	r := run(t, "client", "write", "--url", url, "--start", "100", "--values", "1234")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "function code 6", "[100] 1234 (0x04D2)", "Wrote 1 register(s) to unit 1")
	if got := d.Holding(100, 1); got[0] != 1234 {
		t.Fatalf("holding[100] = %d, want 1234", got[0])
	}
	if d.RequestCount(0x06) != 1 {
		t.Fatalf("expected exactly one FC6 request, got %d", d.RequestCount(0x06))
	}

	// Several values: FC16, with hex and negative notations.
	r = run(t, "client", "write", "--url", url, "--start", "100", "--values", "10,0x00FF,-1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.WriteResult
	decodeJSON(t, r.stdout, &res)
	if res.Function != 16 || res.StartAddress != 100 || res.Count != 3 || res.UnitID != 1 || res.Target != url ||
		!reflect.DeepEqual(res.Registers, []uint16{10, 0xFF, 0xFFFF}) {
		t.Fatalf("unexpected write result: %+v", res)
	}
	if got := d.Holding(100, 3); !reflect.DeepEqual(got, []uint16{10, 0xFF, 0xFFFF}) {
		t.Fatalf("holding[100..102] = %v", got)
	}
	if d.RequestCount(0x10) != 1 {
		t.Fatalf("expected exactly one FC16 request, got %d", d.RequestCount(0x10))
	}

	// Explicit FC16 for a single value.
	r = run(t, "client", "write", "--url", url, "--start", "104", "--function", "16", "--values", "9", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "104", "9")
	if got := d.Holding(104, 1); got[0] != 9 {
		t.Fatalf("holding[104] = %d, want 9", got[0])
	}
}

func TestWrite_codec(t *testing.T) {
	d, url := newDevice(t)
	r := run(t, "client", "write", "--url", url, "--start", "200", "--codec", "float32/layout:4321", "--values", "21.5", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.WriteResult
	decodeJSON(t, r.stdout, &res)
	if res.Function != 16 || res.Codec != "float32/layout:4321" || !reflect.DeepEqual(res.Registers, []uint16{0x41AC, 0}) {
		t.Fatalf("unexpected codec write result: %+v", res)
	}
	if got := d.Holding(200, 2); !reflect.DeepEqual(got, []uint16{0x41AC, 0}) {
		t.Fatalf("holding[200..201] = %#v, want float32 21.5", got)
	}

	// What was written decodes back to the same value.
	r = run(t, "client", "decode", "--url", url, "--start", "200", "--codec", "float32/layout:4321", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var dec types.DecodeResult
	decodeJSON(t, r.stdout, &dec)
	if len(dec.Values) != 1 || dec.Values[0] != 21.5 {
		t.Fatalf("decoded %v, want [21.5]", dec.Values)
	}

	wantKind(t, run(t, "client", "write", "--url", url, "--start", "200", "--codec", "float32/layout:4321", "--values", "abc"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "200", "--codec", "nope", "--values", "1"), errs.KindInvalidInput)
}

func TestWrite_coils(t *testing.T) {
	d, url := newDevice(t)

	r := run(t, "client", "write", "--url", url, "--function", "5", "--start", "20", "--values", "on")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "function code 5")
	if got := d.Coils(20, 1); !got[0] {
		t.Fatal("coil 20 was not switched on")
	}

	r = run(t, "client", "write", "--url", url, "--function", "15", "--start", "30", "--values", "1,0,1,1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.WriteResult
	decodeJSON(t, r.stdout, &res)
	if res.Function != 15 || res.Count != 4 || !reflect.DeepEqual(res.Coils, []bool{true, false, true, true}) {
		t.Fatalf("unexpected coil write result: %+v", res)
	}
	if got := d.Coils(30, 4); !reflect.DeepEqual(got, []bool{true, false, true, true}) {
		t.Fatalf("coils[30..33] = %v", got)
	}

	r = run(t, "client", "write", "--url", url, "--function", "5", "--start", "20", "--values", "off", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "coil")
	if got := d.Coils(20, 1); got[0] {
		t.Fatal("coil 20 was not switched off")
	}
}

func TestWrite_failures(t *testing.T) {
	d, url := newDevice(t)

	r := run(t, "client", "write", "--url", url, "--values", "1234")
	wantKind(t, r, errs.KindInvalidInput)
	wantContains(t, r.err.Error(), "--start")

	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--values", "70000"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--values", "abc"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--function", "3", "--values", "1"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--function", "5", "--values", "maybe"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--function", "5", "--values", "1,0"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--function", "6", "--values", "1,2"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--start", "100", "--values", "1"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "write", "--url", url, "--start", "100", "--values", "1", "--format", "xml"), errs.KindInvalidInput)
	if n := len(d.Requests()); n != 0 {
		t.Fatalf("invalid writes must not reach the device, saw %d requests", n)
	}
	if got := d.Holding(100, 1); got[0] != 1 {
		t.Fatalf("holding[100] changed to %d by a rejected write", got[0])
	}

	wantKind(t, run(t, "client", "write", "--url", refusedURL(t), "--start", "100", "--values", "1"), errs.KindTransport)

	// A device exception fails the write, and the request is not retried.
	d.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Exception: mbtest.ExIllegalDataAddress} }
	r = run(t, "client", "write", "--url", url, "--start", "100", "--values", "5")
	wantKind(t, r, errs.KindModbus)
	wantExit(t, r, errs.ExitProtocol)
	if n := d.RequestCount(0x06); n != 1 {
		t.Fatalf("a failed write must be sent exactly once, saw %d requests", n)
	}
}

func TestWrite_addressFromEnv(t *testing.T) {
	d, url := newDevice(t)
	// With MODBUSCTL_ADDRESS set, --start may be omitted (the value was bound at start-up).
	t.Setenv("MODBUSCTL_ADDRESS", "0")
	r := run(t, "client", "write", "--url", url, "--values", "77", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.WriteResult
	decodeJSON(t, r.stdout, &res)
	if got := d.Holding(res.StartAddress, 1); got[0] != 77 {
		t.Fatalf("holding[%d] = %d, want 77", res.StartAddress, got[0])
	}
}

func TestWrite_transports(t *testing.T) {
	t.Run("udp", func(t *testing.T) {
		d := mbtest.NewDevice().SetHolding(0, 0, 0)
		r := run(t, "client", "write", "--url", "udp://"+d.ServeUDP(t), "--start", "0", "--values", "7,8")
		wantExit(t, r, errs.ExitOK)
		if got := d.Holding(0, 2); !reflect.DeepEqual(got, []uint16{7, 8}) {
			t.Fatalf("holding = %v", got)
		}
	})
	t.Run("rtuovertcp", func(t *testing.T) {
		d := mbtest.NewDevice().SetHolding(0, 0)
		r := run(t, "client", "write", "--url", "rtuovertcp://"+d.ServeRTUOverTCP(t), "--unit", "3", "--start", "0", "--values", "42")
		wantExit(t, r, errs.ExitOK)
		if got := d.Holding(0, 1); got[0] != 42 {
			t.Fatalf("holding = %v", got)
		}
		if reqs := d.Requests(); len(reqs) != 1 || reqs[0].Unit != 3 {
			t.Fatalf("unexpected requests: %+v", reqs)
		}
	})
	t.Run("asciiovertcp", func(t *testing.T) {
		d := mbtest.NewDevice().SetCoils(0, false)
		r := run(t, "client", "write", "--url", "asciiovertcp://"+d.ServeASCIIOverTCP(t), "--function", "5", "--start", "0", "--values", "true")
		wantExit(t, r, errs.ExitOK)
		if got := d.Coils(0, 1); !got[0] {
			t.Fatal("coil 0 was not switched on")
		}
	})
	t.Run("rtu serial", func(t *testing.T) {
		d := mbtest.NewDevice().SetHolding(0, 0, 0)
		path := d.ServeRTUSerial(t)
		r := run(t, "client", "write", "--url", "rtu://"+path, "--baud", "9600", "--parity", "even", "--unit", "3",
			"--start", "0", "--values", "1234,5678", "--format", "json")
		wantExit(t, r, errs.ExitOK)
		var res types.WriteResult
		decodeJSON(t, r.stdout, &res)
		if res.Function != 16 || res.UnitID != 3 || res.Target != "rtu://"+path {
			t.Fatalf("unexpected write result: %+v", res)
		}
		if got := d.Holding(0, 2); !reflect.DeepEqual(got, []uint16{1234, 5678}) {
			t.Fatalf("holding = %v", got)
		}
		// Read back over the same serial line.
		out := filepath.Join(t.TempDir(), "serial.mbcap")
		r = run(t, "client", "read", "--url", "rtu://"+path, "--baud", "9600", "--parity", "even", "--unit", "3",
			"--start", "0", "--count", "2", "--output", out, "--format", "json")
		wantExit(t, r, errs.ExitOK)
		var rd types.ReadResult
		decodeJSON(t, r.stdout, &rd)
		if rd.RawDataHex != "04 D2 16 2E" {
			t.Fatalf("read back %q", rd.RawDataHex)
		}
	})
	t.Run("ascii serial", func(t *testing.T) {
		d := mbtest.NewDevice().SetHolding(0, 0)
		path := d.ServeASCIISerial(t)
		r := run(t, "client", "write", "--url", "ascii://"+path, "--baud", "9600", "--data-bits", "7", "--parity", "odd", "--stop-bits", "1",
			"--start", "0", "--values", "99")
		wantExit(t, r, errs.ExitOK)
		if got := d.Holding(0, 1); got[0] != 99 {
			t.Fatalf("holding = %v", got)
		}
	})
	t.Run("serial settings are validated", func(t *testing.T) {
		wantKind(t, run(t, "client", "write", "--url", "rtu:///dev/null", "--parity", "mark", "--start", "0", "--values", "1"), errs.KindInvalidInput)
		wantKind(t, run(t, "client", "write", "--url", "rtu:///dev/null", "--data-bits", "9", "--start", "0", "--values", "1"), errs.KindInvalidInput)
		wantKind(t, run(t, "client", "write", "--url", "rtu:///dev/null", "--stop-bits", "3", "--start", "0", "--values", "1"), errs.KindInvalidInput)
	})
}

// pollLines splits the JSON Lines output of "client poll --format json".
func pollLines(t *testing.T, stdout string) []types.PollSample {
	t.Helper()
	var samples []types.PollSample
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var s types.PollSample
		decodeJSON(t, line, &s)
		samples = append(samples, s)
	}
	return samples
}

func TestPoll_formats(t *testing.T) {
	d, url := newDevice(t)

	r := run(t, "client", "poll", "--url", url, "--start", "100", "--count", "3", "--polls", "3", "--interval", "1")
	wantExit(t, r, errs.ExitOK)
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 sample lines, got %d:\n%s", len(lines), r.stdout)
	}
	for i, line := range lines {
		wantContains(t, line, fmt.Sprintf("#%d", i+1), "100=1", "101=2", "102=3")
	}
	wantContains(t, r.stderr, "3 poll(s) of "+url, "3 succeeded, 0 failed")
	if n := d.RequestCount(0x03); n != 3 {
		t.Fatalf("expected 3 FC3 requests, got %d", n)
	}

	r = run(t, "client", "poll", "--url", url, "--start", "100", "--count", "2", "--polls", "2", "--interval", "1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	samples := pollLines(t, r.stdout)
	if len(samples) != 2 {
		t.Fatalf("expected 2 JSON lines, got %d:\n%s", len(samples), r.stdout)
	}
	for i, s := range samples {
		if s.Poll != uint32(i+1) || s.UnitID != 1 || s.Function != 3 || s.StartAddress != 100 || s.Count != 2 ||
			s.RawDataHex != "00 01 00 02" || s.Error != nil || !reflect.DeepEqual(s.Values, []any{1.0, 2.0}) {
			t.Fatalf("unexpected sample %d: %+v", i, s)
		}
		if _, err := time.Parse(time.RFC3339Nano, s.Timestamp); err != nil {
			t.Fatalf("sample timestamp %q: %v", s.Timestamp, err)
		}
	}
	wantContains(t, r.stderr, "2 succeeded, 0 failed")

	r = run(t, "client", "poll", "--url", url, "--start", "100", "--count", "2", "--polls", "2", "--interval", "1", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "poll", "timestamp", "rtt_ms", "100", "101", "error")
	if got := strings.Count(r.stdout, "\n"); got != 4 {
		t.Fatalf("expected header, separator and 2 rows, got %d lines:\n%s", got, r.stdout)
	}
}

func TestPoll_countDefault(t *testing.T) {
	_, url := newDevice(t)

	// Without --codec, --count defaults to one register.
	r := run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--interval", "1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	s := pollLines(t, r.stdout)
	if len(s) != 1 || s[0].Count != 1 || s[0].Codec != "" || !reflect.DeepEqual(s[0].Values, []any{1.0}) {
		t.Fatalf("unexpected samples: %+v", s)
	}

	// With --codec, --count defaults to one codec value (two registers for float32).
	r = run(t, "client", "poll", "--url", url, "--start", "103", "--codec", "float32/layout:4321", "--polls", "1", "--interval", "1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	s = pollLines(t, r.stdout)
	if len(s) != 1 || s[0].Codec != "float32/layout:4321" || s[0].RawDataHex != "41 AC 00 00" || !reflect.DeepEqual(s[0].Values, []any{21.5}) {
		t.Fatalf("unexpected codec samples: %+v", s)
	}

	// An explicit --count 0 without a codec is rejected rather than defaulted.
	wantKind(t, run(t, "client", "poll", "--url", url, "--start", "100", "--count", "0", "--polls", "1", "--interval", "1"), errs.KindInvalidInput)

	// Coils poll as booleans.
	r = run(t, "client", "poll", "--url", url, "--function", "1", "--start", "8", "--count", "3", "--polls", "1", "--interval", "1", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	s = pollLines(t, r.stdout)
	if len(s) != 1 || s[0].Function != 1 || len(s[0].Values) != 3 {
		t.Fatalf("unexpected coil samples: %+v", s)
	}
}

func TestPoll_partial(t *testing.T) {
	d, url := newDevice(t)
	// Every second poll fails with an exception: the stream continues and the exit code is 7.
	n := 0
	d.OnRequest = func(mbtest.Request) mbtest.Action {
		n++
		if n%2 == 0 {
			return mbtest.Action{Exception: mbtest.ExServerDeviceFailure}
		}
		return mbtest.Action{}
	}
	r := run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "4", "--interval", "1", "--format", "json")
	if r.err != nil {
		t.Fatalf("a failed poll must not be fatal: %v", r.err)
	}
	wantExit(t, r, errs.ExitPartial)
	s := pollLines(t, r.stdout)
	if len(s) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(s))
	}
	for i, sample := range s {
		if failed := i%2 == 1; failed != (sample.Error != nil) {
			t.Fatalf("sample %d error = %+v", i+1, sample.Error)
		}
	}
	if s[1].Error.Kind != string(errs.KindModbus) {
		t.Fatalf("sample error kind %q, want %q", s[1].Error.Kind, errs.KindModbus)
	}
	wantContains(t, r.stderr, "2 succeeded, 2 failed")

	// Text and table render the failed samples as well.
	n = 1
	r = run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--interval", "1")
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "#1", "error:")
	n = 1
	r = run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--interval", "1", "--format", "table")
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "Server Device Failure")
}

func TestPoll_failures(t *testing.T) {
	_, url := newDevice(t)
	wantKind(t, run(t, "client", "poll", "--start", "100", "--polls", "1"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--format", "xml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--codec", "nope"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--function", "7"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "poll", "--url", url, "--start", "100", "--polls", "1", "--interval", "0"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "poll", "--url", refusedURL(t), "--start", "100", "--polls", "1", "--interval", "1"), errs.KindTransport)
}

func TestOutputWriteFailure(t *testing.T) {
	_, url := newDevice(t)
	// A stdout that cannot be written is a fatal output error, for streams and single results.
	r := runTo(t, context.Background(), failWriter{}, "client", "poll", "--url", url, "--start", "100", "--polls", "2", "--interval", "1")
	wantKind(t, r, errs.KindOutput)
	wantExit(t, r, errs.ExitInternal)
	for _, format := range []string{"text", "json", "table"} {
		r = runTo(t, context.Background(), failWriter{}, "client", "write", "--url", url, "--start", "100", "--values", "1", "--format", format)
		wantKind(t, r, errs.KindOutput)
	}
}

func TestPoll_interrupted(t *testing.T) {
	_, url := newDevice(t)
	// --polls 0 runs until the context is cancelled (Ctrl-C).
	// (A cancel, like a signal; a deadline would also expire the in-flight request itself.)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(60*time.Millisecond, cancel)
	defer timer.Stop()
	r := runCtx(t, ctx, "client", "poll", "--url", url, "--start", "100", "--interval", "5", "--format", "json")
	if r.err != nil {
		t.Fatalf("an interrupted poll must end cleanly: %v", r.err)
	}
	wantExit(t, r, errs.ExitOK)
	if s := pollLines(t, r.stdout); len(s) < 2 {
		t.Fatalf("expected several samples before the interrupt, got %d", len(s))
	}
	wantContains(t, r.stderr, "poll(s) of "+url)
}

func TestScan(t *testing.T) {
	_, url := newDevice(t)
	tmp := t.TempDir()

	cases := []struct {
		algo  string
		start string
		extra []string
	}{
		{algo: "smart", start: "95"},
		{algo: "deep", start: "95"},
		{algo: "stepped", start: "95", extra: []string{"--step", "5"}},
		{algo: "stepped", start: "95", extra: []string{"--step", "5", "--step-half-offset", "--retry-timeout", "1"}},
		// boundary grows from a known-good seed.
		{algo: "boundary", start: "95", extra: []string{"--seed-start", "101", "--seed-count", "2"}},
	}
	for i, tc := range cases {
		algo := tc.algo
		out := filepath.Join(tmp, fmt.Sprintf("%s-%d.mbcap", algo, i))
		args := append([]string{"client", "scan", "--url", url, "--start", tc.start, "--end", "110", "--algo", algo, "--output", out, "--format", "json"}, tc.extra...)
		r := run(t, args...)
		if r.err != nil {
			t.Fatalf("scan --algo %s: %v", algo, r.err)
		}
		// The scan probes addresses the device does not have, so the result is partial.
		wantExit(t, r, errs.ExitPartial)
		var res types.ScanSummaryResult
		decodeJSON(t, r.stdout, &res)
		if res.Algo != algo || res.Target != url || res.CaptureOutputPath != out {
			t.Fatalf("scan --algo %s: unexpected result %+v", algo, res)
		}
		// Refining algos re-read overlapping blocks, so only smart has an exact register count.
		if res.RegistersCaptured < 5 || res.BlocksCaptured < 1 || (algo == "smart" && res.RegistersCaptured != 5) {
			t.Fatalf("scan --algo %s captured %d registers in %d blocks, want the 5 registers at 100-104", algo, res.RegistersCaptured, res.BlocksCaptured)
		}
		if res.SuccessCount+res.FailCount != res.TotalRequests || res.ExceptionCount != res.FailCount || res.FailCount == 0 {
			t.Fatalf("scan --algo %s: inconsistent counters %+v", algo, res)
		}
		if res.Summary == nil || res.Summary.Failed != res.FailCount {
			t.Fatalf("scan --algo %s: summary %+v", algo, res.Summary)
		}
		wantFile(t, out)
		wantContains(t, r.stderr, "algo: "+algo)
	}

	// A range the device fully covers is a clean success (safe is the default algo).
	out := filepath.Join(tmp, "full.mbcap")
	r := run(t, "client", "scan", "--url", url, "--start", "100", "--end", "104", "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Algo: safe", "Registers captured: 5", "Failed: 0", out)
	wantContains(t, r.stderr, "Block: Start: 100, End: 103, Count: 4", "Block: Start: 104, End: 104, Count: 1")
	wantFile(t, out)

	// The safe algo over a range with gaps finds the block and reports the failed probes.
	out = filepath.Join(tmp, "safe-gaps.mbcap")
	r = run(t, "client", "scan", "--url", url, "--start", "95", "--end", "110", "--algo", "safe", "--output", out)
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "Algo: safe", "Exceptions:", out)
	wantContains(t, r.stderr, "Block: Start: 100")
	wantFile(t, out)

	r = run(t, "client", "scan", "--url", url, "--start", "100", "--end", "104", "--algo", "smart", "--output", filepath.Join(tmp, "table.mbcap"), "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "smart")

	wantKind(t, run(t, "client", "scan", "--url", url, "--algo", "boundary", "--output", filepath.Join(tmp, "b.mbcap")), errs.KindInvalidInput)

	// linear reads whole blocks: it captures a fully mapped range in one request.
	r = run(t, "client", "scan", "--url", url, "--start", "100", "--end", "104", "--algo", "linear", "--output", filepath.Join(tmp, "linear.mbcap"), "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var lin types.ScanSummaryResult
	decodeJSON(t, r.stdout, &lin)
	if lin.Algo != "linear" || lin.RegistersCaptured != 5 || lin.FailCount != 0 {
		t.Fatalf("unexpected linear scan: %+v", lin)
	}

	// Input registers, with a delay between requests.
	out = filepath.Join(tmp, "input.mbcap")
	r = run(t, "client", "scan", "--url", url, "--function", "4", "--start", "0", "--end", "1", "--algo", "smart", "--delay", "1", "--output", out, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.ScanSummaryResult
	decodeJSON(t, r.stdout, &res)
	if res.RegistersCaptured != 2 || res.FailCount != 0 {
		t.Fatalf("unexpected input scan: %+v", res)
	}
}

func TestScan_sunspec(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(40000, 0x5375, 0x6E53, 1, 4, 11, 12, 13, 14, 103, 2, 21, 22, 0xFFFF, 0)
	url := "tcp://" + d.ServeTCP(t)
	out := filepath.Join(t.TempDir(), "sunspec.mbcap")
	r := run(t, "client", "scan", "--url", url, "--algo", "sunspec", "--sunspec-base", "40000", "--output", out, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.ScanSummaryResult
	decodeJSON(t, r.stdout, &res)
	if res.Algo != "sunspec" || res.RegistersCaptured < 12 || res.FailCount != 0 {
		t.Fatalf("unexpected sunspec scan: %+v", res)
	}
	wantContains(t, r.stderr, "Block: Start: 40004, End: 40007, Count: 4", "Block: Start: 40010, End: 40011, Count: 2", "Block: Start: 40012, End: 40013, Count: 2")
	wantFile(t, out)

	// Probing the default bases hits addresses the device does not have: partial.
	r = run(t, "client", "scan", "--url", url, "--algo", "sunspec", "--output", filepath.Join(t.TempDir(), "s.mbcap"), "--format", "json")
	wantExit(t, r, errs.ExitPartial)
	decodeJSON(t, r.stdout, &res)
	if res.RegistersCaptured != 14 || res.FailCount == 0 {
		t.Fatalf("unexpected sunspec scan over default bases: %+v", res)
	}
}

func TestScan_failures(t *testing.T) {
	_, url := newDevice(t)
	out := filepath.Join(t.TempDir(), "x.mbcap")
	wantKind(t, run(t, "client", "scan", "--url", url, "--algo", "bogus", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "scan", "--url", url, "--start", "200", "--end", "100", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "scan", "--url", url, "--function", "9", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "scan", "--url", url, "--format", "xml", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "scan", "--start", "1", "--end", "2", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "scan", "--url", refusedURL(t), "--start", "1", "--end", "2", "--output", out), errs.KindTransport)
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("a failed scan must not leave %s behind", out)
	}
}

func TestRecord(t *testing.T) {
	d, url := newDevice(t)
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(`[{"start_address":100,"register_count":3},{"start_address":103,"register_count":2}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(tmp, "rec.mbcap")
	r := run(t, "client", "record", "--url", url, "--blocks-file", blocks, "--interval", "10", "--duration", "25", "--output", out, "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.RecordSummaryResult
	decodeJSON(t, r.stdout, &res)
	if res.Target != url || res.CaptureOutputPath != out || res.Iterations < 1 || res.BlocksRecorded != 2*int(res.Iterations) {
		t.Fatalf("unexpected record result: %+v", res)
	}
	if n := d.RequestCount(0x03); n != res.BlocksRecorded {
		t.Fatalf("device saw %d reads, result reports %d blocks", n, res.BlocksRecorded)
	}
	wantFile(t, out)
	wantContains(t, r.stderr, "Recorded block: Start 100, Count 3", "Recorded block: Start 103, Count 2")

	out = filepath.Join(tmp, "rec-text.mbcap")
	r = run(t, "client", "record", "--url", url, "--blocks-file", blocks, "--interval", "10", "--duration", "15", "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Total recorded blocks:", "Total iterations:", out)
	wantFile(t, out)

	out = filepath.Join(tmp, "rec-table.mbcap")
	r = run(t, "client", "record", "--url", url, "--blocks-file", blocks, "--interval", "10", "--duration", "15", "--output", out, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "blocks_recorded", "iterations", out)
	wantFile(t, out)
}

func TestRecord_failures(t *testing.T) {
	_, url := newDevice(t)
	tmp := t.TempDir()
	out := filepath.Join(tmp, "x.mbcap")
	blocks := filepath.Join(tmp, "blocks.json")
	if err := os.WriteFile(blocks, []byte(`[{"start_address":100,"register_count":3}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(tmp, "bad.json")
	if err := os.WriteFile(bad, []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(tmp, "empty.json")
	if err := os.WriteFile(empty, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}

	// --blocks-file is a required flag: cobra rejects the command before RunE.
	r := run(t, "client", "record", "--url", url, "--output", out)
	if r.err == nil || !strings.Contains(r.err.Error(), "blocks-file") {
		t.Fatalf("expected a required-flag error, got %v", r.err)
	}

	wantKind(t, run(t, "client", "record", "--url", url, "--blocks-file", filepath.Join(tmp, "missing.json"), "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--url", url, "--blocks-file", tmp, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--url", url, "--blocks-file", bad, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--url", url, "--blocks-file", empty, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--url", url, "--blocks-file", blocks, "--format", "xml", "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--blocks-file", blocks, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "record", "--url", refusedURL(t), "--blocks-file", blocks, "--interval", "10", "--duration", "15", "--output", out), errs.KindTransport)
}

// identDevice returns a device with FC43 and FC17 data that only answers unit 1.
func identDevice(t *testing.T) (*mbtest.Device, string) {
	t.Helper()
	d, _ := newDevice(t)
	d.Identification = map[uint8]string{0: "Acme", 1: "P-1000", 2: "1.2.3"}
	d.ServerID = &mbtest.ServerID{ID: []byte{0x42}, Running: true}
	d.Units = []uint8{1}
	return d, "tcp://" + d.ServeTCP(t)
}

func TestIdentify(t *testing.T) {
	_, url := identDevice(t)

	r := run(t, "client", "identify", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Units requested: 1  succeeded: 1  failed: 0", "Object 0: Acme [VendorName]", "Object 1: P-1000 [ProductCode]", "Object 2: 1.2.3 [MajorMinorRevision]")

	r = run(t, "client", "identify", "--url", url, "--format", "json", "--server-id")
	wantExit(t, r, errs.ExitOK)
	var res types.IdentifyResult
	decodeJSON(t, r.stdout, &res)
	if res.Target != url || len(res.Units) != 1 || res.Summary == nil || res.Summary.Succeeded != 1 || res.Summary.Failed != 0 {
		t.Fatalf("unexpected identify result: %+v", res)
	}
	u := res.Units[0]
	if u.UnitID != 1 || u.Error != nil || len(u.Objects) != 3 || u.Objects[0].Value != "Acme" || u.Objects[0].Description != "VendorName" ||
		u.Objects[1].Value != "P-1000" || u.Objects[2].Value != "1.2.3" {
		t.Fatalf("unexpected unit result: %+v", u)
	}
	if u.ReportServerID == nil || u.ReportServerID.DataHex != "42 FF" || u.ReportServerID.RunIndicatorOn == nil || !*u.ReportServerID.RunIndicatorOn {
		t.Fatalf("unexpected report server id: %+v", u.ReportServerID)
	}

	r = run(t, "client", "identify", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "object_id", "VendorName", "Acme", "P-1000")

	for _, level := range []string{"--basic", "--regular", "--extended"} {
		r = run(t, "client", "identify", "--url", url, level, "--format", "json")
		if r.err != nil {
			t.Fatalf("identify %s: %v", level, r.err)
		}
		decodeJSON(t, r.stdout, &res)
		if len(res.Units) != 1 {
			t.Fatalf("identify %s: %+v", level, res)
		}
	}
}

func TestIdentify_partial(t *testing.T) {
	_, url := identDevice(t)

	// Unit 2 does not answer: the result is still written, with exit code 7.
	r := run(t, "client", "identify", "--url", url, "--unit", "1-2", "--server-id", "--format", "json")
	if r.err != nil {
		t.Fatalf("a failed unit must not be fatal: %v", r.err)
	}
	wantExit(t, r, errs.ExitPartial)
	var res types.IdentifyResult
	decodeJSON(t, r.stdout, &res)
	if len(res.Units) != 2 || res.Summary.Requested != 2 || res.Summary.Succeeded != 1 || res.Summary.Failed != 1 {
		t.Fatalf("unexpected identify result: %+v", res)
	}
	if res.Units[0].UnitID != 1 || res.Units[0].Error != nil || res.Units[1].UnitID != 2 || res.Units[1].Error == nil {
		t.Fatalf("unexpected unit results: %+v", res.Units)
	}
	if res.Units[1].Error.Kind != string(errs.KindModbus) {
		t.Fatalf("unit 2 error kind %q", res.Units[1].Error.Kind)
	}

	r = run(t, "client", "identify", "--url", url, "--unit", "1-2")
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "succeeded: 1  failed: 1", "--- Unit ID 1 ---", "--- Unit ID 2 ---", "Gateway Target Failed To Respond")

	r = run(t, "client", "identify", "--url", url, "--unit", "1,2", "--parallel", "1", "--format", "table")
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "Acme", "error", "Gateway Target Failed To Respond")
}

func TestIdentify_failures(t *testing.T) {
	_, url := identDevice(t)
	wantKind(t, run(t, "client", "identify", "--format", "not-a-format"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "identify"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "identify", "--url", url, "--unit", "abc"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "identify", "--url", url, "--unit", "9-3"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "identify", "--url", url, "--timeout", "10"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "identify", "--url", refusedURL(t)), errs.KindTransport)
}

func TestReportServerID(t *testing.T) {
	_, url := identDevice(t)

	r := run(t, "client", "reportserverid", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Units requested: 1  succeeded: 1  failed: 0", "Data: 42 FF", "Run Indicator: ON")

	r = run(t, "client", "reportserverid", "--url", url, "--unit", "1,2", "--format", "json")
	if r.err != nil {
		t.Fatalf("a failed unit must not be fatal: %v", r.err)
	}
	wantExit(t, r, errs.ExitPartial)
	var res types.ReportServerIDResult
	decodeJSON(t, r.stdout, &res)
	if res.Target != url || len(res.Units) != 2 || res.Summary == nil || res.Summary.Succeeded != 1 || res.Summary.Failed != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if u := res.Units[0]; u.UnitID != 1 || u.Error != nil || u.DataHex != "42 FF" || u.RunIndicatorOn == nil || !*u.RunIndicatorOn {
		t.Fatalf("unexpected unit 1: %+v", u)
	}
	if u := res.Units[1]; u.UnitID != 2 || u.Error == nil || u.DataHex != "" {
		t.Fatalf("unexpected unit 2: %+v", u)
	}

	r = run(t, "client", "reportserverid", "--url", url, "--unit", "1-2", "--format", "table")
	wantExit(t, r, errs.ExitPartial)
	wantContains(t, r.stdout, "run_indicator", "42 FF", "ON", "Gateway Target Failed To Respond")

	wantKind(t, run(t, "client", "reportserverid", "--url", url, "--format", "xml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "reportserverid"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "reportserverid", "--url", url, "--unit", "x"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "reportserverid", "--url", refusedURL(t)), errs.KindTransport)
}

func TestFingerprint(t *testing.T) {
	d, url := identDevice(t)

	r := run(t, "client", "fingerprint", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "Units requested: 1  succeeded: 1  failed: 0", "Unit 1: supported read functions", "Read Holding Registers (0x03)", "Read Coils (0x01)")

	r = run(t, "client", "fingerprint", "--url", url, "--format", "json", "--interval", "1")
	wantExit(t, r, errs.ExitOK)
	var res types.FingerprintResult
	decodeJSON(t, r.stdout, &res)
	if res.Target != url || len(res.Units) != 1 || res.Units[0].UnitID != 1 || res.Units[0].Error != nil {
		t.Fatalf("unexpected fingerprint result: %+v", res)
	}
	got := strings.Join(res.Units[0].SupportedReads, "|")
	wantContains(t, got, "Read Holding Registers (0x03)", "Read Input Registers (0x04)", "Read Coils (0x01)", "Read Discrete Inputs (0x02)", "Encapsulated Interface (0x2B)", "Report Server ID (0x11)")

	r = run(t, "client", "fingerprint", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "unit_id", "function", "Read Holding Registers (0x03)")

	// The connection dies while unit 1 is probed: the probe is reported as interrupted (exit 7).
	d.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Close: true} }
	r = run(t, "client", "fingerprint", "--url", url, "--format", "json")
	if r.err != nil {
		t.Fatalf("an interrupted probe must not be fatal: %v", r.err)
	}
	wantExit(t, r, errs.ExitPartial)
	decodeJSON(t, r.stdout, &res)
	if len(res.Units) != 1 || res.Units[0].Error == nil || !res.Units[0].ProbeInterrupted {
		t.Fatalf("unexpected interrupted fingerprint: %+v", res.Units)
	}
	r = run(t, "client", "fingerprint", "--url", url)
	wantExit(t, r, errs.ExitPartial)
	r = run(t, "client", "fingerprint", "--url", url, "--format", "table")
	wantExit(t, r, errs.ExitPartial)

	wantKind(t, run(t, "client", "fingerprint", "--url", url, "--format", "xml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "fingerprint"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "fingerprint", "--url", url, "--unit", "x"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "fingerprint", "--url", refusedURL(t)), errs.KindTransport)
}

func TestDiagnostic(t *testing.T) {
	d, url := newDevice(t)

	r := run(t, "client", "diagnostic", "--url", url)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "sub-function returnquerydata / 0x0000", "Sub-function: 0x0000 (returnquerydata)")

	r = run(t, "client", "diagnostic", "--url", url, "--data", "0102", "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var res types.DiagnosticResult
	decodeJSON(t, r.stdout, &res)
	want := types.DiagnosticResult{Target: url, UnitID: 1, SubFunction: "returnquerydata", SubFunctionHex: 0, DataHex: "01 02"}
	if res != want {
		t.Fatalf("diagnostic result %+v, want %+v", res, want)
	}

	r = run(t, "client", "diagnostic", "--url", url, "--sub-function", "returnbusmessagecount", "--format", "table")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "returnbusmessagecount")
	if n := d.RequestCount(0x08); n != 3 {
		t.Fatalf("expected 3 FC8 requests, got %d", n)
	}

	wantKind(t, run(t, "client", "diagnostic", "--url", url, "--sub-function", "nope"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "diagnostic", "--url", url, "--data", "zz"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "diagnostic", "--url", url, "--format", "xml"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "diagnostic"), errs.KindInvalidInput)
	wantKind(t, run(t, "client", "diagnostic", "--url", refusedURL(t)), errs.KindTransport)

	d.OnRequest = func(mbtest.Request) mbtest.Action { return mbtest.Action{Exception: mbtest.ExIllegalFunction} }
	r = run(t, "client", "diagnostic", "--url", url)
	wantKind(t, r, errs.KindModbus)
	wantExit(t, r, errs.ExitProtocol)
}

func TestClientCommandTree(t *testing.T) {
	want := []string{"codecs", "decode", "diagnostic", "fingerprint", "identify", "poll", "read", "record", "reportserverid", "scan", "sunspec", "write"}
	var got []string
	for _, c := range ClientCmd.Commands() {
		got = append(got, c.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("client subcommands %v, want %v", got, want)
	}

	// Flags stay at their defaults between invocations of the helper.
	_, url := newDevice(t)
	wantExit(t, run(t, "client", "write", "--url", url, "--start", "100", "--values", "5"), errs.ExitOK)
	wantKind(t, run(t, "client", "write", "--url", url, "--values", "5"), errs.KindInvalidInput)

	// The sunspec subtree is reachable from the client command.
	d := mbtest.NewDevice().SetHolding(40000, 0x5375, 0x6E53, 1, 2, 0, 0, 0xFFFF, 0)
	r := run(t, "client", "sunspec", "detect", "--url", "tcp://"+d.ServeTCP(t), "--format", "json")
	wantExit(t, r, errs.ExitOK)
	var det types.SunSpecDetectOutput
	decodeJSON(t, r.stdout, &det)
	if !det.Detected || det.BaseAddress != 40000 {
		t.Fatalf("unexpected sunspec detect: %+v", det)
	}

	r = run(t, "client", "--help")
	wantExit(t, r, errs.ExitOK)
	wantContains(t, r.stdout, "poll", "write", "sunspec")
}

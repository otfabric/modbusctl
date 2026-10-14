package mcap

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-modbus-capture/capture"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/types"
)

// recordCapture records holding registers 100-106 of an in-memory device a few times and
// returns the capture path. The registers hold the text "Hello!  ", 5000 (50.00 Hz) and the
// float32 21.5.
func recordCapture(t *testing.T) string {
	t.Helper()
	d := mbtest.NewDevice().SetHolding(100, 0x4865, 0x6C6C, 0x6F21, 0x2020, 5000, 0x41AC, 0x0000)
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	writeFile(t, blocks, `[{"start_address":100,"register_count":7}]`)
	out := filepath.Join(tmp, "capture.mbcap")
	res, err := modbus.RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: "tcp://" + d.ServeTCP(t), Unit: 1},
		Function:     3,
		BlocksFile:   blocks,
		Interval:     10,
		Duration:     25,
		OutputFile:   out,
	}, io.Discard)
	if err != nil {
		t.Fatalf("record capture: %v", err)
	}
	if res.Iterations < 1 {
		t.Fatalf("capture has no iterations: %+v", res)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// captureStdout runs fn with os.Stdout redirected and returns what was written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = orig }()
	fn()
	os.Stdout = orig
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return readFile(t, path)
}

func TestInfo(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "info.txt")
	r := run(t, "mcap", "info", "--input", in, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "(mbcap)", "Producer:  modbusctl", "Mode:      full capture", "Functions: FC03 x",
		"Targets:", "tcp 127.0.0.1:", "unit 1", "Streams:", "target 1  poll", "every ")

	// Without --output the report goes to stdout.
	var res cmdResult
	stdout := captureStdout(t, func() { res = run(t, "mcap", "info", "--input", in) })
	wantExit(t, res, errs.ExitOK)
	wantContains(t, stdout, "Targets:", "Streams:")
}

func TestConvert(t *testing.T) {
	in := recordCapture(t)
	tmp := t.TempDir()

	// jsonl and csv are lossless: converting back gives the same capture.
	jl := filepath.Join(tmp, "capture.mbcap.jsonl")
	wantExit(t, run(t, "mcap", "convert", "--input", in, "--format", "JSONL", "--output", jl), errs.ExitOK)
	wantContains(t, readFile(t, jl), `{"t":"header","version":"0.1"`, `"req":"0300640007","resp":"030e48656c6c6f212020138841ac0000"`)
	cs := filepath.Join(tmp, "capture.mbcap.csv")
	wantExit(t, run(t, "mcap", "convert", "--input", jl, "--format", "csv", "--output", cs), errs.ExitOK)
	back := filepath.Join(tmp, "back.mbcap")
	wantExit(t, run(t, "mcap", "convert", "--input", cs, "--format", "mbcap", "--output", back), errs.ExitOK)
	if d := capture.Diff(capfiletest.Load(t, in).Capture, capfiletest.Load(t, back).Capture); d != "" {
		t.Fatalf("mbcap > jsonl > csv > mbcap changed the capture: %s", d)
	}

	wide := filepath.Join(tmp, "registers.wide.csv")
	wantExit(t, run(t, "mcap", "convert", "--input", in, "--format", "wide-csv", "--output", wide), errs.ExitOK)
	rows, err := csv.NewReader(strings.NewReader(readFile(t, wide))).ReadAll()
	if err != nil {
		t.Fatalf("wide view is not CSV: %v", err)
	}
	if got := strings.Join(rows[0], ","); got != "at,1/03/100,1/03/101,1/03/102,1/03/103,1/03/104,1/03/105,1/03/106" {
		t.Fatalf("wide columns %q", got)
	}
	if got := strings.Join(rows[1][1:], ","); got != "18533,27756,28449,8224,5000,16812,0" {
		t.Fatalf("wide values %q", got)
	}

	var res cmdResult
	stdout := captureStdout(t, func() { res = run(t, "mcap", "convert", "--input", in, "--format", "jsonl") })
	wantExit(t, res, errs.ExitOK)
	wantContains(t, stdout, `{"t":"target","id":1`)

	// The v2 json export is gone.
	wantKind(t, run(t, "mcap", "convert", "--input", in, "--format", "json"), errs.KindInvalidInput)
}

func TestVerify(t *testing.T) {
	in := recordCapture(t)
	res := run(t, "mcap", "verify", "--input", in)
	wantExit(t, res, errs.ExitOK)
	wantContains(t, res.stdout, ": ok (mbcap, 1 targets, 1 streams, ")

	data := readFile(t, in)
	cut := filepath.Join(t.TempDir(), "cut.mbcap")
	writeFile(t, cut, data[:len(data)-30])
	wantKind(t, run(t, "mcap", "verify", "--input", cut), errs.KindOutput)
}

func TestExtract(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "blocks.json")
	r := run(t, "mcap", "extract", "--input", in, "--output", out)
	wantExit(t, r, errs.ExitOK)
	var blocks []types.AddressBlock
	decodeJSON(t, readFile(t, out), &blocks)
	if len(blocks) != 1 || blocks[0].StartAddress != 100 || blocks[0].RegisterCount != 7 {
		t.Fatalf("unexpected address blocks: %+v", blocks)
	}
}

func TestStrings(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "strings.txt")
	r := run(t, "mcap", "strings", "--input", in, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "[ABCD] [100-103] 4 regs: Hello!")
}

func TestFrequencies(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "freq.txt")
	r := run(t, "mcap", "frequencies", "--input", in, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "[104] 1 regs (uint16 BE) = 5000.0000", "candidate frequency")
}

func TestDecode(t *testing.T) {
	in := recordCapture(t)
	tmp := t.TempDir()
	profile := filepath.Join(tmp, "profile.json")
	writeFile(t, profile, `{"protocolData":{"registers":[
		{"controlledPropertyId":"grid.frequency","valueScaleFactor":0.01,"start":104,"size":1,"format":">H"},
		{"controlledPropertyId":"not.used","start":100,"size":4,"format":">H"},
		{"controlledPropertyId":"temperature","valueScaleFactor":1,"start":105,"size":2,"format":">f"}]}}`)
	out := filepath.Join(tmp, "decoded.txt")
	r := run(t, "mcap", "decode", "--input", in, "--profile", profile, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "Decoded Values:", "grid.frequency = 50.000000", "temperature = 21.500000")

	bad := filepath.Join(tmp, "bad-profile.json")
	writeFile(t, bad, `{"protocolData":{"registers":[{"controlledPropertyId":"x","start":104,"size":1,"format":"??"}]}}`)
	empty := filepath.Join(tmp, "empty-profile.json")
	writeFile(t, empty, `{"protocolData":{"registers":[]}}`)
	wantKind(t, run(t, "mcap", "decode", "--input", in, "--profile", bad, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "mcap", "decode", "--input", in, "--profile", empty, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "mcap", "decode", "--input", in, "--profile", filepath.Join(tmp, "missing.json"), "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "mcap", "decode", "--input", in, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "mcap", "decode", "--input", filepath.Join(tmp, "missing.mbcap"), "--profile", profile, "--output", out), errs.KindInvalidInput)
}

func TestFailures(t *testing.T) {
	in := recordCapture(t)
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "missing.mbcap")
	garbage := filepath.Join(tmp, "garbage.mbcap")
	writeFile(t, garbage, "this is not a capture file")
	profile := filepath.Join(tmp, "profile.json")
	writeFile(t, profile, `{"protocolData":{"registers":[{"controlledPropertyId":"f","valueScaleFactor":1,"start":104,"size":1,"format":">H"}]}}`)
	out := filepath.Join(tmp, "out.txt")
	unwritable := filepath.Join(tmp, "no-such-dir", "out.txt")

	subs := map[string][]string{
		"info":        nil,
		"extract":     nil,
		"strings":     nil,
		"frequencies": nil,
		"convert":     {"--format", "csv"},
		"decode":      {"--profile", profile},
	}
	for sub, extra := range subs {
		args := func(input, output string) []string {
			a := append([]string{"mcap", sub, "--input", input}, extra...)
			if output != "" {
				a = append(a, "--output", output)
			}
			return a
		}
		// A missing or unreadable input is rejected before anything is written.
		r := run(t, args(missing, out)...)
		wantKind(t, r, errs.KindInvalidInput)
		wantExit(t, r, errs.ExitInvalidInput)
		wantKind(t, run(t, args(tmp, out)...), errs.KindInvalidInput)

		// A file that is not MCAP fails while loading.
		r = run(t, args(garbage, out)...)
		wantKind(t, r, errs.KindOutput)
		wantExit(t, r, errs.ExitInternal)

		// An output path in a directory that does not exist cannot be created.
		r = run(t, args(in, unwritable)...)
		if r.err == nil {
			t.Fatalf("mcap %s: expected an error for an unwritable output", sub)
		}
		if _, ok := errs.As(r.err); !ok {
			t.Fatalf("mcap %s: unwritable output gave an untyped error: %v", sub, r.err)
		}

		// A directory-style output passes validation but cannot be created as a file.
		r = run(t, args(in, tmp+string(filepath.Separator))...)
		wantKind(t, r, errs.KindOutput)
		wantExit(t, r, errs.ExitInternal)
	}
	wantKind(t, run(t, "mcap", "convert", "--input", garbage, "--format", "jsonl", "--output", out), errs.KindOutput)

	// --input is a required flag on every subcommand but decode.
	for _, sub := range []string{"info", "extract", "strings", "frequencies", "convert"} {
		r := run(t, "mcap", sub, "--format", "csv")
		if r.err == nil {
			t.Fatalf("mcap %s without --input must fail", sub)
		}
	}
	r := run(t, "mcap", "convert", "--input", in)
	if r.err == nil || !strings.Contains(r.err.Error(), "format") {
		t.Fatalf("expected a required-flag error for --format, got %v", r.err)
	}
	wantKind(t, run(t, "mcap", "convert", "--input", in, "--format", "xml", "--output", out), errs.KindInvalidInput)
}

// A capture with several targets needs --target for the register-block
// commands; a v1 file is refused. Both are invalid input with a stable code.
func TestCaptureErrorCodes(t *testing.T) {
	tmp := t.TempDir()
	c := &capture.Capture{
		Header:  capture.Header{TickNS: 1000, WallStart: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)},
		Targets: []capture.Target{{ID: 1, Unit: 1}, {ID: 2, Unit: 2}},
		Streams: []capture.Stream{{ID: 1, Target: 1}, {ID: 2, Target: 2}},
		Samples: []capture.Sample{
			{Stream: 1, Request: []byte{0x03, 0, 0, 0, 1}, Response: []byte{0x03, 2, 0, 1}},
			{Stream: 2, Request: []byte{0x03, 0, 0, 0, 1}, Response: []byte{0x03, 2, 0, 2}},
		},
	}
	two := capfiletest.Write(t, filepath.Join(tmp, "two.mbcap"), c)
	v1 := filepath.Join(tmp, "old.mcap")
	writeFile(t, v1, "MCAP\x01\x00\x02{}")
	code := func(r cmdResult) string {
		e, ok := errs.As(r.err)
		if !ok {
			t.Fatalf("untyped error %v", r.err)
		}
		return e.Code
	}
	for _, sub := range []string{"strings", "frequencies", "extract"} {
		r := run(t, "mcap", sub, "--input", two)
		wantExit(t, r, errs.ExitInvalidInput)
		if got := code(r); got != errs.CodeCaptureSelectionAmbiguous {
			t.Fatalf("mcap %s: code %s", sub, got)
		}
		wantContains(t, r.err.Error(), "--target 1 --function 3", "--target 2 --function 3")
		wantExit(t, run(t, "mcap", sub, "--input", two, "--target", "2"), errs.ExitOK)
		r = run(t, "mcap", sub, "--input", two, "--target", "2", "--function", "4")
		if got := code(r); got != errs.CodeCaptureSelectionEmpty {
			t.Fatalf("mcap %s: code %s", sub, got)
		}
	}
	for _, sub := range []string{"info", "verify", "strings"} {
		r := run(t, "mcap", sub, "--input", v1)
		wantExit(t, r, errs.ExitInvalidInput)
		if got := code(r); got != errs.CodeCaptureLegacyFormat {
			t.Fatalf("mcap %s of a v1 file: code %s", sub, got)
		}
	}
	wantKind(t, run(t, "mcap", "strings", "--input", two, "--function", "9"), errs.KindInvalidInput)
}

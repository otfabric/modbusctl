// SPDX-License-Identifier: MIT

package capture

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

func TestExtract(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "blocks.json")
	r := run(t, "capture", "extract", "--input", in, "--output", out)
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
	r := run(t, "capture", "strings", "--input", in, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "[ABCD] [100-103] 4 regs: Hello!")
}

func TestFrequencies(t *testing.T) {
	in := recordCapture(t)
	out := filepath.Join(t.TempDir(), "freq.txt")
	r := run(t, "capture", "frequencies", "--input", in, "--output", out)
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
	r := run(t, "capture", "decode", "--input", in, "--profile", profile, "--output", out)
	wantExit(t, r, errs.ExitOK)
	wantContains(t, readFile(t, out), "Decoded Values:", "grid.frequency = 50.000000", "temperature = 21.500000")

	bad := filepath.Join(tmp, "bad-profile.json")
	writeFile(t, bad, `{"protocolData":{"registers":[{"controlledPropertyId":"x","start":104,"size":1,"format":"??"}]}}`)
	empty := filepath.Join(tmp, "empty-profile.json")
	writeFile(t, empty, `{"protocolData":{"registers":[]}}`)
	wantKind(t, run(t, "capture", "decode", "--input", in, "--profile", bad, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "capture", "decode", "--input", in, "--profile", empty, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "capture", "decode", "--input", in, "--profile", filepath.Join(tmp, "missing.json"), "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "capture", "decode", "--input", in, "--output", out), errs.KindInvalidInput)
	wantKind(t, run(t, "capture", "decode", "--input", filepath.Join(tmp, "missing.mbcap"), "--profile", profile, "--output", out), errs.KindInvalidInput)
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
		"extract":     nil,
		"strings":     nil,
		"frequencies": nil,
		"decode":      {"--profile", profile},
	}
	for sub, extra := range subs {
		args := func(input, output string) []string {
			a := append([]string{"capture", sub, "--input", input}, extra...)
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

		// A file that is not a capture fails while loading.
		r = run(t, args(garbage, out)...)
		wantKind(t, r, errs.KindInvalidInput)
		wantExit(t, r, errs.ExitInvalidInput)

		// An output path in a directory that does not exist cannot be created.
		r = run(t, args(in, unwritable)...)
		if r.err == nil {
			t.Fatalf("capture %s: expected an error for an unwritable output", sub)
		}
		if _, ok := errs.As(r.err); !ok {
			t.Fatalf("capture %s: unwritable output gave an untyped error: %v", sub, r.err)
		}

		// A directory-style output passes validation but cannot be created as a file.
		r = run(t, args(in, tmp+string(filepath.Separator))...)
		wantKind(t, r, errs.KindOutput)
		wantExit(t, r, errs.ExitInternal)
	}

	// --input is a required flag on every subcommand but decode.
	for _, sub := range []string{"extract", "strings", "frequencies"} {
		r := run(t, "capture", sub)
		if r.err == nil {
			t.Fatalf("capture %s without --input must fail", sub)
		}
	}
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
	v1 := filepath.Join(tmp, "old.mbcap")
	writeFile(t, v1, "MCAP\x01\x00\x02{}")
	code := func(r cmdResult) string {
		e, ok := errs.As(r.err)
		if !ok {
			t.Fatalf("untyped error %v", r.err)
		}
		return e.Code
	}
	for _, sub := range []string{"strings", "frequencies", "extract"} {
		r := run(t, "capture", sub, "--input", two)
		wantExit(t, r, errs.ExitInvalidInput)
		if got := code(r); got != errs.CodeCaptureSelectionAmbiguous {
			t.Fatalf("capture %s: code %s", sub, got)
		}
		wantContains(t, r.err.Error(), "--target 1 --function 3", "--target 2 --function 3")
		wantExit(t, run(t, "capture", sub, "--input", two, "--target", "2"), errs.ExitOK)
		r = run(t, "capture", sub, "--input", two, "--target", "2", "--function", "4")
		if got := code(r); got != errs.CodeCaptureSelectionEmpty {
			t.Fatalf("capture %s: code %s", sub, got)
		}
	}
	for _, sub := range []string{"extract", "strings", "frequencies"} {
		r := run(t, "capture", sub, "--input", v1)
		wantExit(t, r, errs.ExitInvalidInput)
		if got := code(r); got != errs.CodeCaptureLegacyFormat {
			t.Fatalf("capture %s of a v1 file: code %s", sub, got)
		}
	}
	wantKind(t, run(t, "capture", "strings", "--input", two, "--function", "9"), errs.KindInvalidInput)
}

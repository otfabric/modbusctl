package mcap

import (
	"context"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/modbus"
	"github.com/otfabric/modbusctl/internal/types"
)

// recordCapture records holding registers 100-106 of an in-memory device a few times and
// returns the MCAP path. The registers hold the text "Hello!  ", 5000 (50.00 Hz) and the
// float32 21.5.
func recordCapture(t *testing.T) string {
	t.Helper()
	d := mbtest.NewDevice().SetHolding(100, 0x4865, 0x6C6C, 0x6F21, 0x2020, 5000, 0x41AC, 0x0000)
	tmp := t.TempDir()
	blocks := filepath.Join(tmp, "blocks.json")
	writeFile(t, blocks, `[{"start_address":100,"register_count":7}]`)
	out := filepath.Join(tmp, "capture.mcap")
	res, err := modbus.RecordAndWriteMCAP(context.Background(), config.RecordConfig{
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
	wantContains(t, readFile(t, out), "Header Information:", "IP:        127.0.0.1", "Unit:      1", "Function:  3",
		"Record Summary:", "Iteration 0:", "Total Registers: 7", "Address Range: 100 → 106")

	// Without --output the report goes to stdout.
	var res cmdResult
	stdout := captureStdout(t, func() { res = run(t, "mcap", "info", "--input", in) })
	wantExit(t, res, errs.ExitOK)
	wantContains(t, stdout, "Header Information:", "Address Range: 100 → 106")
}

func TestConvert(t *testing.T) {
	in := recordCapture(t)
	tmp := t.TempDir()

	out := filepath.Join(tmp, "capture.csv")
	r := run(t, "mcap", "convert", "--input", in, "--format", "csv", "--output", out)
	wantExit(t, r, errs.ExitOK)
	rows, err := csv.NewReader(strings.NewReader(readFile(t, out))).ReadAll()
	if err != nil {
		t.Fatalf("output is not CSV: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected a header and at least one record, got %d rows", len(rows))
	}
	wantHeader := "ip,port,unit,function,iteration,request_timestamp,response_timestamp,start_address,register_count,data_hex"
	if got := strings.Join(rows[0], ","); got != wantHeader {
		t.Fatalf("CSV header %q, want %q", got, wantHeader)
	}
	for i, row := range rows[1:] {
		if row[0] != "127.0.0.1" || row[2] != "1" || row[3] != "3" || row[7] != "100" || row[8] != "7" || row[9] != "48656c6c6f212020138841ac0000" {
			t.Fatalf("unexpected CSV row %d: %v", i, row)
		}
	}

	// The format name is case-insensitive.
	out = filepath.Join(tmp, "capture.json")
	r = run(t, "mcap", "convert", "--input", in, "--format", "JSON", "--output", out)
	wantExit(t, r, errs.ExitOK)
	var doc struct {
		Header struct {
			IP       string `json:"ip"`
			Unit     uint8  `json:"unit"`
			Function uint8  `json:"function"`
		} `json:"header"`
		Records []struct {
			Iteration     int    `json:"iteration"`
			StartAddress  uint16 `json:"start_address"`
			RegisterCount uint16 `json:"register_count"`
			RawData       string `json:"raw_data"`
		} `json:"records"`
	}
	decodeJSON(t, readFile(t, out), &doc)
	if doc.Header.IP != "127.0.0.1" || doc.Header.Unit != 1 || doc.Header.Function != 3 || len(doc.Records) != len(rows)-1 {
		t.Fatalf("unexpected JSON export: %+v", doc)
	}
	for i, rec := range doc.Records {
		if rec.Iteration != i || rec.StartAddress != 100 || rec.RegisterCount != 7 || rec.RawData != "48656c6c6f212020138841ac0000" {
			t.Fatalf("unexpected record %d: %+v", i, rec)
		}
	}

	var res cmdResult
	stdout := captureStdout(t, func() { res = run(t, "mcap", "convert", "--input", in, "--format", "csv") })
	wantExit(t, res, errs.ExitOK)
	wantContains(t, stdout, wantHeader, "48656c6c6f212020138841ac0000")
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
	wantKind(t, run(t, "mcap", "decode", "--input", filepath.Join(tmp, "missing.mcap"), "--profile", profile, "--output", out), errs.KindInvalidInput)
}

func TestFailures(t *testing.T) {
	in := recordCapture(t)
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "missing.mcap")
	garbage := filepath.Join(tmp, "garbage.mcap")
	writeFile(t, garbage, "this is not an MCAP file")
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
	wantKind(t, run(t, "mcap", "convert", "--input", garbage, "--format", "json", "--output", out), errs.KindOutput)

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

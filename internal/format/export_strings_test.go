package format

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestExportStrings(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_export_strings_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "10.0.0.4",
		Port:      502,
		Unit:      1,
		Function:  3,
		StartTime: time.Now().UnixNano(),
	}
	if err := mcap.WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 100, RegisterCount: 4, Data: []byte("TEST")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 200, RegisterCount: 2, Data: []byte{0x00, 0xFF}},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 300, RegisterCount: 6, Data: []byte("HELLO!")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 400, RegisterCount: 8, Data: append([]byte{0x00, 0x01, 0x02}, []byte("EMBED")...)},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 500, RegisterCount: 10, Data: append(append([]byte{0x00, 0x01, 0x02}, []byte("HELLO")...), 0xFF, 0xFE)},
	}

	for _, rec := range records {
		if err := mcap.AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	var buf bytes.Buffer
	cfg := config.StringsConfig{
		InputFile: tmpfile.Name(),
	}

	if err := ExportStrings(&buf, cfg); err != nil {
		t.Fatalf("ExportStrings failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "TEST") || !strings.Contains(output, "HELLO!") || !strings.Contains(output, "EMBED") || !strings.Contains(output, "HELLO") {
		t.Errorf("Output missing expected ASCII strings:\n%s", output)
	}
	if strings.Contains(output, "ÿ") || strings.Contains(output, "\x00") {
		t.Errorf("Output contains non-ASCII content:\n%s", output)
	}
}

func TestExportStringsWithSplitASCII(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_export_strings_split_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "10.0.0.5",
		Port:      502,
		Unit:      1,
		Function:  3,
		StartTime: time.Now().UnixNano(),
	}
	if err := mcap.WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 100, RegisterCount: 3, Data: []byte("HELLO ")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 103, RegisterCount: 3, Data: []byte("CRAZY ")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 106, RegisterCount: 3, Data: []byte("WORLD!")},
	}

	for _, rec := range records {
		if err := mcap.AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	var buf bytes.Buffer
	cfg := config.StringsConfig{
		InputFile: tmpfile.Name(),
	}
	if err := ExportStrings(&buf, cfg); err != nil {
		t.Fatalf("ExportStrings failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "HELLO CRAZY WORLD!") {
		t.Errorf("Expected combined string 'HELLO CRAZY WORLD!' not found in output:\n%s", output)
	}
}

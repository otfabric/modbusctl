package format

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestExportStrings(t *testing.T) {
	path := capfiletest.WriteRecords(t, capfiletest.Path(t, "strings.mbcap"),
		types.CaptureHeader{
			IP:        "10.0.0.4",
			Port:      502,
			Unit:      1,
			Function:  3,
			StartTime: time.Now().UnixNano(),
		},
		[]types.CaptureRecord{
			{Iteration: 0, StartAddress: 100, RegisterCount: 4, Data: []byte("TEST")},
			{Iteration: 0, StartAddress: 200, RegisterCount: 2, Data: []byte{0x00, 0xFF}},
			{Iteration: 0, StartAddress: 300, RegisterCount: 6, Data: []byte("HELLO!")},
			{Iteration: 0, StartAddress: 400, RegisterCount: 8, Data: append([]byte{0x00, 0x01, 0x02}, []byte("EMBED")...)},
			{Iteration: 0, StartAddress: 500, RegisterCount: 10, Data: append(append([]byte{0x00, 0x01, 0x02}, []byte("HELLO")...), 0xFF, 0xFE)},
		}...)

	var buf bytes.Buffer
	cfg := config.StringsConfig{
		InputFile: path,
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
	path := capfiletest.WriteRecords(t, capfiletest.Path(t, "strings.mbcap"),
		types.CaptureHeader{
			IP:        "10.0.0.5",
			Port:      502,
			Unit:      1,
			Function:  3,
			StartTime: time.Now().UnixNano(),
		},
		[]types.CaptureRecord{
			{Iteration: 0, StartAddress: 100, RegisterCount: 3, Data: []byte("HELLO ")},
			{Iteration: 0, StartAddress: 103, RegisterCount: 3, Data: []byte("CRAZY ")},
			{Iteration: 0, StartAddress: 106, RegisterCount: 3, Data: []byte("WORLD!")},
		}...)

	var buf bytes.Buffer
	cfg := config.StringsConfig{
		InputFile: path,
	}
	if err := ExportStrings(&buf, cfg); err != nil {
		t.Fatalf("ExportStrings failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "HELLO CRAZY WORLD!") {
		t.Errorf("Expected combined string 'HELLO CRAZY WORLD!' not found in output:\n%s", output)
	}
}

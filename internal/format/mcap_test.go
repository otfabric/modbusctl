package format

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestWriteAndReadHeader(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_test_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	original := types.CaptureHeader{
		IP:        "127.0.0.1",
		Port:      502,
		Unit:      1,
		Function:  4,
		StartTime: time.Now().UnixNano(),
	}

	if err := WriteHeader(tmpfile, original); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Seek back to start to read
	if _, err := tmpfile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	result, err := ReadHeader(tmpfile)
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	if result.IP != original.IP || result.Port != original.Port || result.Unit != original.Unit {
		t.Errorf("Header mismatch: got %+v, want %+v", result, original)
	}
}

func TestAppendReadAndCountRecords(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_test_records_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "192.168.1.100",
		Port:      502,
		Unit:      1,
		Function:  4,
		StartTime: time.Now().UnixNano(),
	}

	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 100, RegisterCount: 2, Data: []byte{0x01, 0x02, 0x03, 0x04}},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 200, RegisterCount: 1, Data: []byte{0x05, 0x06}},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 300, RegisterCount: 3, Data: []byte{0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C}},
	}

	for _, rec := range records {
		if err := AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	// Seek to start before reading
	if _, err := tmpfile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	_, err = ReadHeader(tmpfile)
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	for i := 0; i < len(records); i++ {
		readRec, err := ReadRecord(tmpfile)
		if err != nil {
			t.Fatalf("ReadRecord #%d failed: %v", i, err)
		}
		if readRec.StartAddress != records[i].StartAddress || readRec.RegisterCount != records[i].RegisterCount {
			t.Errorf("Record #%d mismatch: got %+v, want %+v", i, readRec, records[i])
		}
		if len(readRec.Data) != len(records[i].Data) {
			t.Errorf("Record #%d data length mismatch", i)
		}
	}

	// Validate CountRecords
	count, err := CountRecords(tmpfile.Name())
	if err != nil {
		t.Fatalf("CountRecords failed: %v", err)
	}
	if count != len(records) {
		t.Errorf("Expected %d records, got %d", len(records), count)
	}
}

func TestMillionRecordsPerformance(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_perf_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Test file: %s", tmpfile.Name())
	defer func() { _ = tmpfile.Close() }()
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	header := types.CaptureHeader{
		IP:        "127.0.0.1",
		Port:      502,
		Unit:      1,
		Function:  3,
		StartTime: time.Now().UnixNano(),
	}
	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	// Create a fixed 10-register data block (20 bytes)
	data := make([]byte, 20)
	for i := range data {
		data[i] = byte(i)
	}

	t.Log("Writing 1,000,000 records...")
	start := time.Now()
	var i uint32
	for i = 0; i < uint32(1_000_000); i++ {
		now := time.Now().UnixNano()
		rec := types.CaptureRecord{
			Iteration:         i,
			RequestTimestamp:  now,
			ResponseTimestamp: now,
			StartAddress:      1000,
			RegisterCount:     10,
			Data:              data,
		}
		if err := AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord #%d failed: %v", i, err)
		}
	}
	elapsedWrite := time.Since(start)
	t.Logf("Write completed in %v", elapsedWrite)

	info, err := tmpfile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("File size: %.2f MB", float64(info.Size())/(1024*1024))

	t.Log("Counting records...")
	if _, err := tmpfile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	count, err := CountRecords(tmpfile.Name())
	if err != nil {
		t.Fatalf("CountRecords failed: %v", err)
	}
	elapsedRead := time.Since(start)
	t.Logf("Counted %d records in %v", count, elapsedRead)
}

func TestExportCSV(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_export_csv_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "10.0.0.1",
		Port:      502,
		Unit:      1,
		Function:  3,
		StartTime: time.Now().UnixNano(),
	}
	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 10, RegisterCount: 2, Data: []byte{0x01, 0x02, 0x03, 0x04}},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 20, RegisterCount: 1, Data: []byte{0x05, 0x06}},
	}

	for _, rec := range records {
		if err := AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, tmpfile.Name()); err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ip,port,unit,function,iteration,request_timestamp,response_timestamp,start_address,register_count,data_hex") {
		t.Error("CSV header missing or incorrect")
	}
	if !strings.Contains(output, "10.0.0.1") {
		t.Error("CSV does not contain expected IP")
	}
	if !strings.Contains(output, "10") || !strings.Contains(output, "20") {
		t.Error("CSV missing expected start addresses")
	}
}

func TestExportJSON(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_export_json_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "10.0.0.2",
		Port:      502,
		Unit:      2,
		Function:  4,
		StartTime: time.Now().UnixNano(),
	}
	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 30, RegisterCount: 1, Data: []byte{0x0A, 0x0B}},
	}

	for _, rec := range records {
		if err := AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := ExportJSON(&buf, tmpfile.Name()); err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"ip": "10.0.0.2"`) {
		t.Error("JSON output missing header IP")
	}
	if !strings.Contains(output, `"start_address": 30`) {
		t.Error("JSON output missing record data")
	}
}

func TestExportAddressBlocks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "mcap_export_blocks_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()
	defer func() { _ = tmpfile.Close() }()

	header := types.CaptureHeader{
		IP:        "10.0.0.3",
		Port:      502,
		Unit:      3,
		Function:  4,
		StartTime: time.Now().UnixNano(),
	}
	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 40, RegisterCount: 2, Data: []byte{0x01, 0x02, 0x03, 0x04}},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 50, RegisterCount: 1, Data: []byte{0x05, 0x06}},
		{Iteration: 1, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 40, RegisterCount: 2, Data: []byte{0x07, 0x08, 0x09, 0x0A}}, // duplicate block
	}

	for _, rec := range records {
		if err := AppendRecord(tmpfile, rec); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := ExportAddressBlocks(&buf, tmpfile.Name()); err != nil {
		t.Fatalf("ExportAddressBlocks failed: %v", err)
	}

	var blocks []types.AddressBlock
	if err := json.Unmarshal(buf.Bytes(), &blocks); err != nil {
		t.Fatalf("Failed to parse exported JSON: %v", err)
	}

	found40 := false
	found50 := false
	for _, b := range blocks {
		if b.StartAddress == 40 && b.RegisterCount == 2 {
			found40 = true
		}
		if b.StartAddress == 50 && b.RegisterCount == 1 {
			found50 = true
		}
	}

	if !found40 {
		t.Error("Output does not contain expected address block 40 with count 2")
	}
	if !found50 {
		t.Error("Output does not contain expected address block 50 with count 1")
	}
	if len(blocks) != 2 {
		t.Errorf("Expected 2 unique address blocks, got %d", len(blocks))
	}
}
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
	if err := WriteHeader(tmpfile, header); err != nil {
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
		if err := AppendRecord(tmpfile, rec); err != nil {
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

func TestExportInfo(t *testing.T) {
	cases := []struct {
		name     string
		records  []types.CaptureRecord
		expected []string
	}{
		{
			name: "1 iteration, 1 block",
			records: []types.CaptureRecord{
				{Iteration: 0, RequestTimestamp: time.Now().UnixNano(), ResponseTimestamp: time.Now().UnixNano(), StartAddress: 100, RegisterCount: 1, Data: []byte{0x01, 0x02}},
			},
			expected: []string{"Iterations: 1", "Blocks: 1", "Total Registers: 1", "Address Range: 100 → 100"},
		},
		{
			name: "1 iteration, multiple blocks",
			records: []types.CaptureRecord{
				{Iteration: 0, RequestTimestamp: time.Now().UnixNano(), ResponseTimestamp: time.Now().UnixNano(), StartAddress: 100, RegisterCount: 2, Data: []byte{0x01, 0x02, 0x03, 0x04}},
				{Iteration: 0, RequestTimestamp: time.Now().Add(1 * time.Second).UnixNano(), ResponseTimestamp: time.Now().Add(1 * time.Second).UnixNano(), StartAddress: 102, RegisterCount: 2, Data: []byte{0x05, 0x06, 0x07, 0x08}},
			},
			expected: []string{"Iterations: 1", "Blocks: 2", "Total Registers: 4", "Address Range: 100 → 103"},
		},
		{
			name: "multiple iterations, 1 block each",
			records: []types.CaptureRecord{
				{Iteration: 0, RequestTimestamp: time.Now().UnixNano(), ResponseTimestamp: time.Now().UnixNano(), StartAddress: 200, RegisterCount: 1, Data: []byte{0x01, 0x02}},
				{Iteration: 1, RequestTimestamp: time.Now().Add(1 * time.Second).UnixNano(), ResponseTimestamp: time.Now().Add(1 * time.Second).UnixNano(), StartAddress: 300, RegisterCount: 2, Data: []byte{0x03, 0x04, 0x05, 0x06}},
			},
			expected: []string{"Iterations: 2", "Iteration 0:", "Iteration 1:", "Blocks: 1", "Total Registers: 1", "Total Registers: 2"},
		},
		{
			name: "multiple iterations, multiple blocks",
			records: []types.CaptureRecord{
				{Iteration: 0, RequestTimestamp: time.Now().UnixNano(), ResponseTimestamp: time.Now().UnixNano(), StartAddress: 400, RegisterCount: 2, Data: []byte{0x01, 0x02, 0x03, 0x04}},
				{Iteration: 0, RequestTimestamp: time.Now().Add(1 * time.Second).UnixNano(), ResponseTimestamp: time.Now().Add(1 * time.Second).UnixNano(), StartAddress: 402, RegisterCount: 2, Data: []byte{0x05, 0x06, 0x07, 0x08}},
				{Iteration: 1, RequestTimestamp: time.Now().Add(2 * time.Second).UnixNano(), ResponseTimestamp: time.Now().Add(2 * time.Second).UnixNano(), StartAddress: 500, RegisterCount: 1, Data: []byte{0x09, 0x0A}},
				{Iteration: 1, RequestTimestamp: time.Now().Add(3 * time.Second).UnixNano(), ResponseTimestamp: time.Now().Add(3 * time.Second).UnixNano(), StartAddress: 502, RegisterCount: 3, Data: []byte{0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}},
			},
			expected: []string{"Iterations: 2", "Iteration 0:", "Iteration 1:", "Blocks: 2", "Total Registers: 4", "Total Registers: 4", "Address Range: 400 → 403", "Address Range: 500 → 504"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tmpfile, err := os.CreateTemp("", "mcap_info_*.mcap")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(tmpfile.Name()) }()
			defer func() { _ = tmpfile.Close() }()

			header := types.CaptureHeader{
				IP:        "192.168.9.99",
				Port:      502,
				Unit:      1,
				Function:  3,
				StartTime: time.Now().UnixNano(),
			}
			if err := WriteHeader(tmpfile, header); err != nil {
				t.Fatalf("WriteHeader failed: %v", err)
			}

			for _, rec := range c.records {
				if err := AppendRecord(tmpfile, rec); err != nil {
					t.Fatalf("AppendRecord failed: %v", err)
				}
			}

			var buf bytes.Buffer
			if err := ExportInfo(&buf, tmpfile.Name()); err != nil {
				t.Fatalf("ExportInfo failed: %v", err)
			}

			output := buf.String()
			for _, expect := range c.expected {
				if !strings.Contains(output, expect) {
					t.Errorf("Expected output to contain %q\nFull output:\n%s", expect, output)
				}
			}
		})
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
	if err := WriteHeader(tmpfile, header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}

	now := time.Now().UnixNano()
	records := []types.CaptureRecord{
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 100, RegisterCount: 3, Data: []byte("HELLO ")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 103, RegisterCount: 3, Data: []byte("CRAZY ")},
		{Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now, StartAddress: 106, RegisterCount: 3, Data: []byte("WORLD!")},
	}

	for _, rec := range records {
		if err := AppendRecord(tmpfile, rec); err != nil {
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

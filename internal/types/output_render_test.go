package types

import (
	"strings"
	"testing"
)

func TestReadResult_textAndTable(t *testing.T) {
	t.Parallel()
	r := &ReadResult{
		Target:         "t",
		UnitID:         1,
		Function:       3,
		StartAddress:   40001,
		RegisterCount:  2,
		RawByteCount:   4,
		PreSwapHex:     "aabb",
		RawDataHex:     "bbaa",
		BytesSwapped:   true,
		AsciiDecoded:   "OK",
		McapOutputPath: "/tmp/x.mcap",
	}
	s, err := r.MarshalTextOutput()
	if err != nil || !strings.Contains(s, "Byte-swapped") || !strings.Contains(s, "ASCII") {
		t.Fatalf("text: %v %q", err, s)
	}
	h := r.TableHeaders()
	if len(h) != 2 {
		t.Fatal(h)
	}
	rows := r.TableRows()
	if len(rows) < 8 {
		t.Fatalf("rows %d", len(rows))
	}
	var nilR *ReadResult
	if _, err := nilR.MarshalTextOutput(); err != nil {
		t.Fatal(err)
	}
	if nilR.TableRows() != nil {
		t.Fatal("nil rows")
	}
}

func TestScanSummaryResult_textAndTable(t *testing.T) {
	t.Parallel()
	r := &ScanSummaryResult{
		Target:              "tcp://h:502",
		Summary:             &ResultSummary{Requested: 10, Succeeded: 8, Failed: 2},
		Algo:                "safe",
		TotalRequests:       10,
		SuccessCount:        8,
		FailCount:           2,
		ExceptionCount:      1,
		TimeoutCount:        1,
		TransportErrorCount: 0,
		BlocksCaptured:      3,
		RegistersCaptured:   30,
		AvgResponseMs:       12,
		Duration:            "1s",
		McapOutputPath:      "/o.mcap",
	}
	s, err := r.MarshalTextOutput()
	if err != nil || !strings.Contains(s, "Exceptions:") {
		t.Fatalf("text: %v %q", err, s)
	}
	if len(r.TableHeaders()) == 0 || len(r.TableRows()) == 0 {
		t.Fatal("table")
	}
	var nilS *ScanSummaryResult
	if _, err := nilS.MarshalTextOutput(); err != nil {
		t.Fatal(err)
	}
	if nilS.TableRows() != nil {
		t.Fatal("nil scan rows")
	}
}

func TestDiscoverOutput_textAndTable(t *testing.T) {
	t.Parallel()
	empty := &DiscoverOutput{Devices: nil}
	s, err := empty.MarshalTextOutput()
	if err != nil || s != "No Modbus devices found." {
		t.Fatalf("%v %q", err, s)
	}
	with := &DiscoverOutput{
		Port: 502,
		Devices: []DiscoverJson{
			{IP: "10.0.0.1", Port: 502, Mac: "aa:bb"},
			{IP: "10.0.0.2", Port: 502},
		},
	}
	s2, err := with.MarshalTextOutput()
	if err != nil || !strings.Contains(s2, "MAC") || !strings.Contains(s2, "10.0.0.2") {
		t.Fatalf("%v %q", err, s2)
	}
	if len(with.TableHeaders()) != 4 || len(with.TableRows()) != 2 {
		t.Fatal("table discover")
	}
}

func TestDiagnosticResult_textAndTable(t *testing.T) {
	t.Parallel()
	r := &DiagnosticResult{
		Target: "tcp://1.2.3.4:502", UnitID: 1, SubFunction: "returnquerydata",
		SubFunctionHex: 0, DataHex: "00",
	}
	s, err := r.MarshalTextOutput()
	if err != nil || !strings.Contains(s, "FC08") {
		t.Fatalf("%v %q", err, s)
	}
	if len(r.TableRows()) < 4 {
		t.Fatal("rows")
	}
}

func TestRecordSummaryResult_textAndTable(t *testing.T) {
	t.Parallel()
	r := &RecordSummaryResult{
		Target: "t", BlocksRecorded: 3, Iterations: 2, McapOutputPath: "/r.mcap",
	}
	s, err := r.MarshalTextOutput()
	if err != nil || !strings.Contains(s, "blocks") {
		t.Fatalf("%v %q", err, s)
	}
	if len(r.TableRows()) != 4 {
		t.Fatalf("rows %d", len(r.TableRows()))
	}
}

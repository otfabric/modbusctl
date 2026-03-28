package types_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/modbusctl/internal/format"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestIdentifyResult_textGolden(t *testing.T) {
	cat := uint8(1)
	conf := uint8(0x83)
	more := false
	next := uint8(0)
	run := true
	res := &types.IdentifyResult{
		Target: "tcp://192.168.1.1:502",
		Units: []types.IdentifyUnitResult{
			{
				UnitID:          1,
				Category:        &cat,
				ConformityLevel: &conf,
				MoreFollows:     &more,
				NextObjectID:    &next,
				Objects: []types.IdentifyObjectRow{
					{ID: 0, Value: "Acme", Description: "VendorName"},
					{ID: 1, Value: "X", Description: "ProductCode"},
				},
				ReportServerID: &types.IdentifyReportServerOutput{
					DataHex:        "deadbeef",
					RunIndicatorOn: &run,
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatText, res); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "identify_text.golden", buf.Bytes())
}

func TestIdentifyResult_jsonStructure(t *testing.T) {
	res := &types.IdentifyResult{
		Target: "tcp://x:502",
		Units: []types.IdentifyUnitResult{
			{UnitID: 3, Error: types.EmbeddedModbusError("timeout")},
		},
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatJSON, res); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["target"]; !ok {
		t.Fatal("json missing target")
	}
	units, ok := m["units"].([]any)
	if !ok || len(units) != 1 {
		t.Fatalf("json units: got %#v", m["units"])
	}
	u0, ok := units[0].(map[string]any)
	if !ok {
		t.Fatal("unit row not object")
	}
	if _, ok := u0["unit_id"]; !ok {
		t.Fatal("json missing unit_id")
	}
	errObj, ok := u0["error"].(map[string]any)
	if !ok {
		t.Fatalf("json error not object: %#v", u0["error"])
	}
	if _, ok := errObj["message"]; !ok {
		t.Fatal("json error missing message")
	}
}

func TestReportServerIDResult_textGolden(t *testing.T) {
	run1, run2 := false, true
	res := &types.ReportServerIDResult{
		Target: "tcp://192.168.1.1:502",
		Units: []types.ReportServerIDUnitResult{
			{UnitID: 1, DataHex: "aa", RunIndicatorOn: &run1},
			{UnitID: 2, DataHex: "bbbb", RunIndicatorOn: &run2},
		},
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatText, res); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "reportserverid_text.golden", buf.Bytes())
}

func TestReportServerIDResult_jsonStructure(t *testing.T) {
	res := &types.ReportServerIDResult{
		Target: "tcp://x:502",
		Units: []types.ReportServerIDUnitResult{
			{UnitID: 10, Error: types.EmbeddedModbusError("modbus error")},
		},
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatJSON, res); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["target"]; !ok {
		t.Fatal("json missing target")
	}
	units, ok := m["units"].([]any)
	if !ok || len(units) != 1 {
		t.Fatalf("json units: got %#v", m["units"])
	}
}

func TestScanSummaryResult_jsonStructure(t *testing.T) {
	res := &types.ScanSummaryResult{
		Target:         "tcp://h:502",
		Algo:           "safe",
		TotalRequests:  10,
		Duration:       "1s",
		McapOutputPath: "/tmp/x.mcap",
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatJSON, res); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"target", "algo", "total_requests", "duration", "mcap_output_path"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing json key %q", key)
		}
	}
}

func TestRecordSummaryResult_jsonStructure(t *testing.T) {
	res := &types.RecordSummaryResult{
		Target:         "tcp://h:502",
		BlocksRecorded: 3,
		Iterations:     2,
		McapOutputPath: "/tmp/r.mcap",
	}
	var buf bytes.Buffer
	if err := format.Write(&buf, format.FormatJSON, res); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"target", "blocks_recorded", "iterations", "mcap_output_path"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing json key %q", key)
		}
	}
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output mismatch %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

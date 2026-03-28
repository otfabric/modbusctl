package format

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/mcap"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestExportHeuristicFrequency_writesCandidates(t *testing.T) {
	t.Parallel()
	tmp, err := os.CreateTemp(t.TempDir(), "freq_*.mcap")
	if err != nil {
		t.Fatal(err)
	}
	path := tmp.Name()
	defer func() { _ = tmp.Close() }()

	hdr := types.CaptureHeader{
		IP: "127.0.0.1", Port: 502, Unit: 1, Function: 3,
		StartTime: time.Now().UnixNano(),
	}
	if err := mcap.WriteHeader(tmp, hdr); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(50.0))
	now := time.Now().UnixNano()
	rec := types.CaptureRecord{
		Iteration: 0, RequestTimestamp: now, ResponseTimestamp: now,
		StartAddress: 40000, RegisterCount: 2, Data: b,
	}
	if err := mcap.AppendRecord(tmp, rec); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()

	var buf bytes.Buffer
	if err := ExportHeuristicFrequency(&buf, path); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("candidate frequency")) {
		t.Fatalf("output: %q", out)
	}
}

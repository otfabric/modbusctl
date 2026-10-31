// SPDX-License-Identifier: MIT

package format

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

func TestExportHeuristicFrequency_writesCandidates(t *testing.T) {
	t.Parallel()
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(50.0))
	path := capfiletest.WriteRecords(t, capfiletest.Path(t, "freq.mbcap"),
		types.CaptureHeader{IP: "127.0.0.1", Port: 502, Unit: 1, Function: 3},
		types.CaptureRecord{StartAddress: 40000, RegisterCount: 2, Data: b})

	var buf bytes.Buffer
	if err := ExportHeuristicFrequency(&buf, path, config.CaptureSelection{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("candidate frequency")) {
		t.Fatalf("output: %q", out)
	}
}

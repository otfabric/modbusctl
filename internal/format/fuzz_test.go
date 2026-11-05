// SPDX-License-Identifier: MIT

package format

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/analysis"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

// FuzzCaptureAnalyses feeds arbitrary capture files, which come from wherever
// the user got them, through capture strings, frequencies and extract. They
// must not panic, and the strings they print are printable ASCII.
func FuzzCaptureAnalyses(f *testing.F) {
	dir := f.TempDir()
	h := types.CaptureHeader{IP: "10.0.0.1", Port: 502, Unit: 1, Function: 3}
	for i, recs := range [][]types.CaptureRecord{
		{{Iteration: 1, StartAddress: 100, RegisterCount: 4, Data: []byte("Hello!\x1b[2J")}},
		{{Iteration: 1, StartAddress: 65532, RegisterCount: 4, Data: []byte{0x13, 0x88, 'A', 'B', 'C', 'D', 0x42, 0x48}}},
		{{Iteration: 1, StartAddress: 0, RegisterCount: 1, Data: []byte{0x13, 0x88}}, {Iteration: 2, StartAddress: 0, RegisterCount: 1, Data: []byte{0x13, 0x89}}},
	} {
		p := capfiletest.WriteRecords(f, filepath.Join(dir, "seed"+string(rune('a'+i))+".mbcap"), h, recs...)
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte(`{"format":"mbcap-jsonl"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "in")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := ExportStrings(&out, config.StringsConfig{InputFile: path}); err == nil {
			for _, line := range strings.Split(out.String(), "\n") {
				for i := 0; i < len(line); i++ {
					if line[i] < 0x20 || line[i] == 0x7f {
						t.Fatalf("control byte %#x in strings output %q", line[i], line)
					}
				}
			}
		}
		_ = ExportHeuristicFrequency(io.Discard, path, config.CaptureSelection{})
		_ = analysis.ExportAddressBlocks(io.Discard, path, config.CaptureSelection{})
	})
}

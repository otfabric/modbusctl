// SPDX-License-Identifier: MIT

package format

import (
	"bytes"
	"strings"
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
)

// Records built from a capture never pass address 65535 (capture.SplitAddresses
// refuses such a sample); the analyses do not report an address past it for one
// that would.
func TestAnalyses_recordsAtTheTopOfTheAddressSpace(t *testing.T) {
	rec := &types.CaptureRecord{StartAddress: 65534, RegisterCount: 4, Data: []byte("ABCDEFGH")}
	var buf bytes.Buffer
	processASCII(&buf, rec)
	if strings.Contains(buf.String(), "[65534-65537]") || strings.Contains(buf.String(), "[65534-1]") {
		t.Fatalf("an address past 65535 was reported:\n%s", buf.String())
	}
	if out := buf.String(); out != "" && !strings.Contains(out, "[65534-65535]") {
		t.Fatalf("output %q", out)
	}

	// 50.00 Hz as a uint16 in the last register, and again at an offset past it.
	freq := &types.CaptureRecord{StartAddress: 65535, RegisterCount: 2, Data: []byte{0x13, 0x88, 0x13, 0x88}}
	for _, m := range processFrequency([]types.CaptureRecord{*freq}) {
		if int(m.Addr)+int(m.Regs)-1 > 65535 || m.Addr < 65535 {
			t.Fatalf("match at %d x%d is past the address space", m.Addr, m.Regs)
		}
	}
}

func TestRenderTable_escapesCells(t *testing.T) {
	out, err := RenderTable(tableOf{headers: []string{"a", "b"}, rows: [][]string{{"x\ty", "\x1b[31mred"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out, "\x1b") || !strings.Contains(out, `x\ty`) || !strings.Contains(out, `\x1b[31mred`) {
		t.Fatalf("table %q", out)
	}
}

type tableOf struct {
	headers []string
	rows    [][]string
}

func (t tableOf) TableHeaders() []string { return t.headers }
func (t tableOf) TableRows() [][]string  { return t.rows }

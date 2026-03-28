package format

import (
	"fmt"
	"io"

	"github.com/otfabric/modbusctl/internal/types"
)

// ASCIIOrder denotes a byte/register layout for interpreting stitched register data.
// ABCD = no transform; BADC = swap bytes within each 16-bit register;
// CDAB = swap register pairs within each 32-bit group; DCBA = both.
type ASCIIOrder string

const (
	ASCIIOrderABCD ASCIIOrder = "ABCD"
	ASCIIOrderBADC ASCIIOrder = "BADC"
	ASCIIOrderCDAB ASCIIOrder = "CDAB"
	ASCIIOrderDCBA ASCIIOrder = "DCBA"
)

// processASCII scans a stitched record's data for printable ASCII strings.
// It automatically tries the common register/byte layouts: ABCD, BADC, CDAB, and DCBA.
// Identical (register range, string) lines are emitted once across layouts to cut noise.
// Output format example: [ABCD] [100-103] 4 regs: TEST.
func processASCII(w io.Writer, rec *types.CaptureRecord) {
	const minASCII = 4
	seen := make(map[string]struct{})

	orders := []ASCIIOrder{
		ASCIIOrderABCD,
		ASCIIOrderBADC,
		ASCIIOrderCDAB,
		ASCIIOrderDCBA,
	}

	for _, order := range orders {
		data := reorderASCIIBytes(rec.Data, order)
		processASCIIForOrder(w, rec, data, order, minASCII, seen)
	}
}

func processASCIIForOrder(w io.Writer, rec *types.CaptureRecord, data []byte, order ASCIIOrder, minASCII int, seen map[string]struct{}) {
	start := -1

	for i := 0; i < len(data); i++ {
		b := data[i]
		if isPrintableASCII(b) {
			if start == -1 {
				start = i
			}
			continue
		}

		if start != -1 && i-start >= minASCII {
			writeASCIICandidate(w, rec, data, order, start, i-1, seen)
		}
		start = -1
	}

	if start != -1 && len(data)-start >= minASCII {
		writeASCIICandidate(w, rec, data, order, start, len(data)-1, seen)
	}
}

func reorderASCIIBytes(src []byte, order ASCIIOrder) []byte {
	if len(src) == 0 {
		return nil
	}

	dst := make([]byte, len(src))
	copy(dst, src)

	// Transform complete 4-byte groups: [A B][C D] => ABCD / BADC / CDAB / DCBA
	for i := 0; i+3 < len(src); i += 4 {
		a, b := src[i], src[i+1]
		c, d := src[i+2], src[i+3]

		switch order {
		case ASCIIOrderABCD:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = a, b, c, d
		case ASCIIOrderBADC:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = b, a, d, c
		case ASCIIOrderCDAB:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = c, d, a, b
		case ASCIIOrderDCBA:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = d, c, b, a
		default:
			dst[i], dst[i+1], dst[i+2], dst[i+3] = a, b, c, d
		}
	}

	// Trailing 1–3 bytes left unchanged (safest for stitched streams where tail may not form a full 32-bit group).
	return dst
}

func writeASCIICandidate(w io.Writer, rec *types.CaptureRecord, data []byte, order ASCIIOrder, startByte, endByte int, seen map[string]struct{}) {
	sub := data[startByte : endByte+1]
	asciiStartReg := rec.StartAddress + uint16(startByte/2)
	asciiEndReg := rec.StartAddress + uint16(endByte/2)
	key := fmt.Sprintf("%d:%d:%s", asciiStartReg, asciiEndReg, string(sub))
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}

	_, _ = fmt.Fprintf(
		w,
		"[%s] [%d-%d] %d regs: %s\n",
		order,
		asciiStartReg,
		asciiEndReg,
		asciiEndReg-asciiStartReg+1,
		string(sub),
	)
}

func isPrintableASCII(b byte) bool {
	return b >= 32 && b <= 126
}

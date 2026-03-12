package format

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/otfabric/modbusctl/internal/types"
)

type StitchedBlock struct {
	Start uint16
	End   uint16
	Data  []byte
}

type decodeAttempt struct {
	Addr       uint16
	Regs       uint16
	Format     string
	Value      float64
	Confidence float64
}

var decoders = []struct {
	Name   string
	Size   int // in bytes
	Decode func([]byte) (float64, error)
}{
	{"uint16 BE", 2, func(b []byte) (float64, error) {
		if len(b) < 2 {
			return 0, io.ErrUnexpectedEOF
		}
		return float64(binary.BigEndian.Uint16(b)), nil
	}},
	{"uint16 LE", 2, func(b []byte) (float64, error) {
		return float64(binary.LittleEndian.Uint16(b)), nil
	}},
	{"int16 BE", 2, func(b []byte) (float64, error) {
		return float64(int16(binary.BigEndian.Uint16(b))), nil
	}},
	{"int16 LE", 2, func(b []byte) (float64, error) {
		return float64(int16(binary.LittleEndian.Uint16(b))), nil
	}},
	{"float32 BE", 4, func(b []byte) (float64, error) {
		bits := binary.BigEndian.Uint32(b)
		return float64(math.Float32frombits(bits)), nil
	}},
	{"float32 LE", 4, func(b []byte) (float64, error) {
		bits := binary.LittleEndian.Uint32(b)
		return float64(math.Float32frombits(bits)), nil
	}},
	{"float64 BE", 8, func(b []byte) (float64, error) {
		bits := binary.BigEndian.Uint64(b)
		return math.Float64frombits(bits), nil
	}},
	{"float64 LE", 8, func(b []byte) (float64, error) {
		bits := binary.LittleEndian.Uint64(b)
		return math.Float64frombits(bits), nil
	}},
}

// stitchAdjacentRecords merges adjacent CaptureRecords into larger blocks if they are contiguous in address space.
func stitchAdjacentRecords(records []types.CaptureRecord) []types.CaptureRecord {
	if len(records) == 0 {
		return nil
	}
	var stitched []types.CaptureRecord
	prev := records[0]

	for i := 1; i < len(records); i++ {
		curr := records[i]
		expectedStart := prev.StartAddress + prev.RegisterCount
		if curr.StartAddress == expectedStart {
			prev.Data = append(prev.Data, curr.Data...)
			prev.RegisterCount += curr.RegisterCount
		} else {
			stitched = append(stitched, prev)
			prev = curr
		}
	}
	stitched = append(stitched, prev)
	return stitched
}

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
// Output format example: [ABCD] [100-103] 4 regs: TEST
func processASCII(w io.Writer, rec *types.CaptureRecord) {
	const minASCII = 4

	orders := []ASCIIOrder{
		ASCIIOrderABCD,
		ASCIIOrderBADC,
		ASCIIOrderCDAB,
		ASCIIOrderDCBA,
	}

	for _, order := range orders {
		data := reorderASCIIBytes(rec.Data, order)
		processASCIIForOrder(w, rec, data, order, minASCII)
	}
}

func processASCIIForOrder(w io.Writer, rec *types.CaptureRecord, data []byte, order ASCIIOrder, minASCII int) {
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
			writeASCIICandidate(w, rec, data, order, start, i-1)
		}
		start = -1
	}

	if start != -1 && len(data)-start >= minASCII {
		writeASCIICandidate(w, rec, data, order, start, len(data)-1)
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

func writeASCIICandidate(w io.Writer, rec *types.CaptureRecord, data []byte, order ASCIIOrder, startByte, endByte int) {
	sub := data[startByte : endByte+1]
	asciiStartReg := rec.StartAddress + uint16(startByte/2)
	asciiEndReg := rec.StartAddress + uint16(endByte/2)

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

// processFrequency scans records for frequency-like values and returns the matches sorted by confidence.
func processFrequency(records []types.CaptureRecord) []decodeAttempt {
	var matches []decodeAttempt

	for _, rec := range records {
		data := rec.Data
		for _, d := range decoders {
			for i := 0; i <= len(data)-d.Size; i++ {
				slice := data[i : i+d.Size]
				val, err := d.Decode(slice)
				if err != nil || math.IsNaN(val) || math.IsInf(val, 0) {
					continue
				}

				conf := frequencyConfidence(val)
				if conf == 0.0 {
					continue
				}

				addr := rec.StartAddress + uint16(i/2)
				matches = append(matches, decodeAttempt{
					Addr:       addr,
					Regs:       uint16(d.Size / 2),
					Format:     d.Name,
					Value:      val,
					Confidence: conf,
				})
			}
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Confidence > matches[j].Confidence
	})

	return matches
}

func frequencyConfidence(val float64) float64 {
	// Native Hz (no scaling)
	if val >= 47.5 && val <= 52.0 {
		return scoreUnscaled(val)
	}

	// DeciHz (e.g., 500 = 50.0 Hz)
	if val >= 475 && val <= 520 {
		return scoreScaled(val / 10)
	}

	// CentiHz (e.g., 5000 = 50.00 Hz)
	if val >= 4750 && val <= 5200 {
		return scoreScaled(val / 100)
	}

	// Other scales could be added here (e.g., milliHz)

	return 0.0
}

func scoreUnscaled(v float64) float64 {
	if math.Abs(v-50.0) < 1e-6 {
		return 0.95 // Give high but not perfect confidence for exact 50.0000
	}
	delta := math.Abs(v - 50.0)
	switch {
	case delta < 0.2:
		return 1.0 - delta/0.2 // 1.0 to 0.0
	case delta < 1.0:
		return 0.6 - (delta-0.2)/0.8*0.4 // 0.6 to 0.2
	case delta < 2.5:
		return 0.3 - (delta-1.0)/1.5*0.2 // 0.3 to 0.1
	default:
		return 0.0
	}
}

func scoreScaled(v float64) float64 {
	return scoreUnscaled(v)
}

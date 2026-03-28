package format

import (
	"encoding/binary"
	"io"
	"math"
	"sort"

	"github.com/otfabric/modbusctl/internal/types"
)

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

// processFrequency scans records for frequency-like values and returns the matches sorted by confidence.
func processFrequency(records []types.CaptureRecord) []decodeAttempt {
	var matches []decodeAttempt

	for _, rec := range records {
		data := rec.Data
		for _, d := range decoders {
			// Register-aligned starting offsets only (reduces overlapping junk from arbitrary byte shifts).
			for i := 0; i <= len(data)-d.Size; i += 2 {
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

	type freqKey struct {
		addr   uint16
		regs   uint16
		format string
	}
	best := make(map[freqKey]decodeAttempt)
	for _, m := range matches {
		k := freqKey{m.Addr, m.Regs, m.Format}
		if old, ok := best[k]; !ok || m.Confidence > old.Confidence {
			best[k] = m
		}
	}
	matches = matches[:0]
	for _, m := range best {
		matches = append(matches, m)
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

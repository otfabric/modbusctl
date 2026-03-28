package format

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
)

func TestProcessFrequency_empty(t *testing.T) {
	t.Parallel()
	if out := processFrequency(nil); len(out) != 0 {
		t.Fatalf("got %v", out)
	}
	if out := processFrequency([]types.CaptureRecord{}); len(out) != 0 {
		t.Fatalf("got %v", out)
	}
}

func TestProcessFrequency_detects50Hz_float32BE(t *testing.T) {
	t.Parallel()
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(50.0))
	rec := types.CaptureRecord{StartAddress: 100, Data: b}
	matches := processFrequency([]types.CaptureRecord{rec})
	if len(matches) == 0 {
		t.Fatal("expected at least one match")
	}
	top := matches[0]
	if top.Confidence < 0.9 || math.Abs(top.Value-50.0) > 1e-3 {
		t.Fatalf("top match: %+v", top)
	}
}

func TestProcessFrequency_deciAndCentiScaling(t *testing.T) {
	t.Parallel()
	// 500 uint16 BE → deciHz interpretation path (500/10 = 50 Hz)
	d500 := make([]byte, 2)
	binary.BigEndian.PutUint16(d500, 500)
	// 5000 uint16 BE → centiHz path
	d5000 := make([]byte, 2)
	binary.BigEndian.PutUint16(d5000, 5000)
	var all []types.CaptureRecord
	for _, d := range [][]byte{d500, d5000} {
		all = append(all, types.CaptureRecord{StartAddress: 0, Data: d})
	}
	matches := processFrequency(all)
	if len(matches) < 2 {
		t.Fatalf("want multiple scale hits, got %d: %+v", len(matches), matches)
	}
}

func TestProcessFrequency_dedupesByAddrRegsFormat(t *testing.T) {
	t.Parallel()
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(50.0))
	matches := processFrequency([]types.CaptureRecord{
		{StartAddress: 0, Data: b},
		{StartAddress: 0, Data: b},
	})
	n := 0
	for _, m := range matches {
		if m.Format == "float32 BE" && m.Addr == 0 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want one float32-BE@0 after dedupe, got %d in %+v", n, matches)
	}
}

func TestFrequencyConfidence_outOfRange(t *testing.T) {
	t.Parallel()
	if frequencyConfidence(10.0) != 0 {
		t.Fatal("out of range")
	}
	if frequencyConfidence(47.4) != 0 {
		t.Fatal("below native hz band")
	}
}

func TestScoreUnscaled_branches(t *testing.T) {
	t.Parallel()
	if s := scoreUnscaled(50.0); math.Abs(s-0.95) > 1e-9 {
		t.Fatalf("exact 50: %v", s)
	}
	if s := scoreUnscaled(49.9); s <= 0 || s >= 1.0 {
		t.Fatalf("49.9: %v", s)
	}
	if s := scoreUnscaled(48.0); s <= 0 {
		t.Fatalf("48.0: %v", s)
	}
	if s := scoreUnscaled(52.0); s <= 0 {
		t.Fatalf("edge 52: %v", s)
	}
	if s := scoreUnscaled(60.0); s != 0 {
		t.Fatalf("far: %v", s)
	}
}

func TestScoreScaled_delegates(t *testing.T) {
	t.Parallel()
	if scoreScaled(50.0) != scoreUnscaled(50.0) {
		t.Fatal("delegate")
	}
}

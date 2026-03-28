package format

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
)

func TestStitchAdjacentRecords_perIteration(t *testing.T) {
	t.Parallel()
	// Same addresses but different iterations must not merge.
	recs := []types.CaptureRecord{
		{Iteration: 1, StartAddress: 0, RegisterCount: 1, Data: []byte{0, 1}},
		{Iteration: 2, StartAddress: 1, RegisterCount: 1, Data: []byte{2, 3}},
	}
	out := stitchAdjacentRecords(recs)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2 (no cross-iteration stitch)", len(out))
	}
}

func TestStitchAdjacentRecords_sortsThenStitches(t *testing.T) {
	t.Parallel()
	recs := []types.CaptureRecord{
		{Iteration: 0, StartAddress: 2, RegisterCount: 1, Data: []byte{0, 1}},
		{Iteration: 0, StartAddress: 0, RegisterCount: 2, Data: []byte{2, 3, 4, 5}},
	}
	out := stitchAdjacentRecords(recs)
	if len(out) != 1 {
		t.Fatalf("len=%d want 1 stitched block", len(out))
	}
	if out[0].StartAddress != 0 || out[0].RegisterCount != 3 {
		t.Fatalf("got start=%d count=%d want 0,3", out[0].StartAddress, out[0].RegisterCount)
	}
}

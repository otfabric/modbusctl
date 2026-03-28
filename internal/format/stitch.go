package format

import (
	"sort"

	"github.com/otfabric/modbusctl/internal/types"
)

// stitchAdjacentRecords merges adjacent capture rows within the same iteration when addresses are contiguous.
// Records are grouped by Iteration, sorted by StartAddress, then stitched so multi-iteration MCAPs cannot merge across iterations.
func stitchAdjacentRecords(records []types.CaptureRecord) []types.CaptureRecord {
	if len(records) == 0 {
		return nil
	}
	byIter := make(map[uint32][]types.CaptureRecord)
	for _, r := range records {
		byIter[r.Iteration] = append(byIter[r.Iteration], r)
	}
	iters := make([]uint32, 0, len(byIter))
	for k := range byIter {
		iters = append(iters, k)
	}
	sort.Slice(iters, func(i, j int) bool { return iters[i] < iters[j] })
	var out []types.CaptureRecord
	for _, iter := range iters {
		grp := byIter[iter]
		sort.Slice(grp, func(i, j int) bool { return grp[i].StartAddress < grp[j].StartAddress })
		out = append(out, stitchContiguousSameIteration(grp)...)
	}
	return out
}

func stitchContiguousSameIteration(sorted []types.CaptureRecord) []types.CaptureRecord {
	if len(sorted) == 0 {
		return nil
	}
	var stitched []types.CaptureRecord
	prev := sorted[0]
	for i := 1; i < len(sorted); i++ {
		curr := sorted[i]
		expectedStart := prev.StartAddress + prev.RegisterCount
		if curr.StartAddress == expectedStart {
			prev.Data = append(prev.Data, curr.Data...)
			prev.RegisterCount += curr.RegisterCount
		} else {
			stitched = append(stitched, prev)
			prev = curr
		}
	}
	return append(stitched, prev)
}

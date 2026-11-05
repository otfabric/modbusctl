// SPDX-License-Identifier: MIT

package modbus

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
)

// TestDeepRefinement_staysInRangeAtTheTopOfTheAddressSpace: a boundary refinement
// window that reaches address 65535 used a 16-bit loop counter, which wrapped to 0
// and queued reads of addresses 0.. far outside the configured range until the
// queue cap stopped it.
func TestDeepRefinement_staysInRangeAtTheTopOfTheAddressSpace(t *testing.T) {
	for _, tc := range []struct {
		name               string
		readable, failed   Interval
		globalStart, glEnd uint16
	}{
		{"right boundary", Interval{Start: 65520, Count: 11}, Interval{Start: 65531, Count: 5}, 65400, 65535},
		{"left boundary", Interval{Start: 65530, Count: 6}, Interval{Start: 65520, Count: 10}, 65400, 65535},
		{"bottom", Interval{Start: 5, Count: 10}, Interval{Start: 0, Count: 5}, 0, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newDeepStrategy(scanSettings{ScanConfig: config.ScanConfig{StartAddress: tc.globalStart, EndAddress: tc.glEnd}})
			s.readableIntervals = []Interval{tc.readable}
			s.failedIntervals = []Interval{tc.failed}
			s.buildRefinementQueue()
			if len(s.refinementQueue) == 0 {
				t.Fatal("no refinement at a boundary with an adjacent failure")
			}
			for _, task := range s.refinementQueue {
				if task.Start < tc.globalStart || uint32(task.Start)+uint32(task.Count)-1 > uint32(tc.glEnd) {
					t.Fatalf("refinement task %d x%d outside the range [%d, %d] (%d tasks)", task.Start, task.Count, tc.globalStart, tc.glEnd, len(s.refinementQueue))
				}
			}
		})
	}
}

// TestStrategies_neverLeaveTheRange runs every address-range algorithm against
// devices with readable islands at both ends of the address space and checks
// that no read leaves the configured range (an address computation that
// wrapped would read at 0 or panic in the checked conversions). Stepped probes
// one address on each side of the range by design (ALGOS.md 5.5).
func TestStrategies_neverLeaveTheRange(t *testing.T) {
	oracles := map[string]func(ScanTask) bool{
		"all readable": func(ScanTask) bool { return true },
		"none":         func(ScanTask) bool { return false },
		"island at the top": func(task ScanTask) bool {
			return task.Start >= 65500 && uint32(task.Start)+uint32(task.Count)-1 <= 65530
		},
		"single registers": func(task ScanTask) bool { return task.Count == 1 && task.Start%3 == 0 },
	}
	ranges := [][2]uint16{{65400, 65535}, {0, 130}, {65530, 65535}}
	for _, algo := range []string{"safe", "smart", "deep", "stepped", "linear", "boundary"} {
		for name, oracle := range oracles {
			for _, r := range ranges {
				cfg := config.ScanConfig{Algo: algo, StartAddress: r[0], EndAddress: r[1], Step: 7, SeedStart: r[0], SeedCount: 4}
				s, err := newScanStrategy(ss(cfg))
				if err != nil {
					t.Fatal(err)
				}
				lo, hi := int(r[0]), int(r[1])
				if algo == "stepped" {
					lo, hi = max(lo-1, 0), min(hi+1, 65535)
				}
				for _, task := range runStrategy(t, s, cfg, oracle, 20000) {
					if int(task.Start) < lo || int(task.Start)+int(task.Count)-1 > hi {
						t.Fatalf("%s, %s, range %v: task %d x%d leaves the range", algo, name, r, task.Start, task.Count)
					}
				}
			}
		}
	}
}

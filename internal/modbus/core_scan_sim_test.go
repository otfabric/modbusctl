// SPDX-License-Identifier: MIT

package modbus

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coreSimMaxTasks bounds a simulated scan; no test here needs anywhere near as many reads.
const coreSimMaxTasks = 20000

// coreRunStrategy drives the strategy selected by ss against a simulated device on which a
// read succeeds when every address it covers is readable. It returns the tasks issued, the
// addresses read successfully, and whether the strategy finished within coreSimMaxTasks.
func coreRunStrategy(t *testing.T, ss scanSettings, readable func(addr int) bool) (tasks []ScanTask, covered map[uint16]bool, finished bool) {
	t.Helper()
	strategy, err := newScanStrategy(ss)
	require.NoError(t, err)
	strategy.Init(ss)

	covered = map[uint16]bool{}
	for !strategy.Done() {
		if len(tasks) >= coreSimMaxTasks {
			return tasks, covered, false
		}
		task, ok := strategy.Next()
		if !ok {
			break
		}
		tasks = append(tasks, task)

		lo, hi := int(task.Start), int(task.Start)+int(task.Count)-1
		success := task.Count >= 1 && task.Count <= MaxBlockSize && hi <= 65535
		for a := lo; success && a <= hi; a++ {
			success = readable(a)
		}
		result := ScanResult{Success: success, Start: task.Start, Count: task.Count, OutcomeType: ScanOutcomeSuccess}
		if success {
			for a := lo; a <= hi; a++ {
				covered[uint16(a)] = true
			}
		} else {
			result.OutcomeType = ScanOutcomeException
			result.ExceptionCode = 0x02
		}
		strategy.OnResult(task, result)
	}
	return tasks, covered, true
}

// coreSimulate is coreRunStrategy for scans that must terminate and only issue reads a
// Modbus device can be asked for: 1 to 125 registers, not running past address 65535.
func coreSimulate(t *testing.T, ss scanSettings, readable func(addr int) bool) ([]ScanTask, map[uint16]bool) {
	t.Helper()
	tasks, covered, finished := coreRunStrategy(t, ss, readable)
	require.True(t, finished, "strategy did not finish within %d reads", coreSimMaxTasks)
	for _, task := range tasks {
		require.True(t, task.Count >= 1 && task.Count <= MaxBlockSize, "invalid read: start=%d count=%d", task.Start, task.Count)
		require.LessOrEqual(t, int(task.Start)+int(task.Count)-1, 65535, "read past the address space: start=%d count=%d", task.Start, task.Count)
	}
	return tasks, covered
}

// coreReadableIn returns a predicate that is true inside ranges.
func coreReadableIn(ranges ...coreRange) func(int) bool {
	return func(a int) bool {
		for _, r := range ranges {
			if a >= r.lo && a <= r.hi {
				return true
			}
		}
		return false
	}
}

// coreWantCovered is the set of readable addresses inside the scanned range.
func coreWantCovered(cfg config.ScanConfig, ranges []coreRange) map[uint16]bool {
	want := map[uint16]bool{}
	for a := range coreAddrSet(ranges...) {
		if a >= cfg.StartAddress && a <= cfg.EndAddress {
			want[a] = true
		}
	}
	return want
}

// The exhaustive strategies must capture every readable register of the range, whatever the
// shape of the register map, and never read outside the range.
func TestCoreStrategies_captureEveryReadableRegister(t *testing.T) {
	maps := map[string][]coreRange{
		"nothing readable":       nil,
		"everything readable":    {{0, 65535}},
		"block at range start":   {{1000, 1300}},
		"single registers":       {{1003, 1003}, {1100, 1100}, {1999, 1999}},
		"blocks and gaps":        {{1010, 1019}, {1030, 1031}, {1200, 1460}, {1990, 1999}},
		"last register only":     {{1999, 1999}},
		"all but first and last": {{1001, 1998}},
	}
	for _, algo := range []string{"smart", "deep"} {
		for name, ranges := range maps {
			t.Run(algo+"/"+name, func(t *testing.T) {
				cfg := config.ScanConfig{Algo: algo, StartAddress: 1000, EndAddress: 1999}
				tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(ranges...))
				assert.Equal(t, coreWantCovered(cfg, ranges), covered)
				for _, task := range tasks {
					assert.GreaterOrEqual(t, task.Start, cfg.StartAddress)
					assert.LessOrEqual(t, int(task.Start)+int(task.Count)-1, int(cfg.EndAddress))
				}
			})
		}
	}

	// Safe reads forward from the start of the range, so a map without a leading gap is
	// captured in maximum-size reads.
	safe := map[string][]coreRange{
		"nothing readable":     nil,
		"everything readable":  {{0, 65535}},
		"block at range start": {{1000, 1300}},
	}
	for name, ranges := range safe {
		t.Run("safe/"+name, func(t *testing.T) {
			cfg := config.ScanConfig{Algo: "safe", StartAddress: 1000, EndAddress: 1999}
			tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(ranges...))
			assert.Equal(t, coreWantCovered(cfg, ranges), covered)
			for _, task := range tasks {
				assert.GreaterOrEqual(t, task.Start, cfg.StartAddress)
				assert.LessOrEqual(t, int(task.Start)+int(task.Count)-1, int(cfg.EndAddress))
			}
		})
	}
	t.Run("safe/everything readable takes maximum-size reads", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "safe", StartAddress: 0, EndAddress: 999}
		tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{0, 999}))
		assert.Len(t, tasks, 8)
		for _, task := range tasks {
			assert.Equal(t, uint16(125), task.Count)
		}
	})
	t.Run("safe/top of the address space", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "safe", StartAddress: 65530, EndAddress: 65535}
		_, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{65530, 65535}))
		assert.Equal(t, coreAddrSet(coreRange{65530, 65535}), covered)
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn())
		assert.Empty(t, covered)
		assert.Len(t, tasks, 3+3+3+2+2+1, "every candidate size at every address")
	})
}

// Linear finds blocks that are at least one maximum read long, with their exact boundaries.
func TestCoreLinearStrategy_denseMaps(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.ScanConfig
		ranges []coreRange
	}{
		{name: "everything readable", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 1000}, ranges: []coreRange{{0, 65535}}},
		{name: "block from the start, tail search", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 1000}, ranges: []coreRange{{0, 200}}},
		{name: "block to the end, backward search", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 1000}, ranges: []coreRange{{300, 1000}}},
		{name: "block in the middle", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 499}, ranges: []coreRange{{100, 349}}},
		{name: "block starting one before a probe", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 999}, ranges: []coreRange{{124, 600}}},
		{name: "block starting exactly on a probe", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 999}, ranges: []coreRange{{125, 600}}},
		{name: "block ending one register after a full read", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 999}, ranges: []coreRange{{0, 125}}},
		{name: "two blocks", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 1999}, ranges: []coreRange{{0, 300}, {700, 1100}}},
		{name: "nothing readable", cfg: config.ScanConfig{StartAddress: 0, EndAddress: 1000}},
		{name: "range shorter than one read", cfg: config.ScanConfig{StartAddress: 10, EndAddress: 19}, ranges: []coreRange{{0, 100}}},
		{name: "top of the address space, all readable", cfg: config.ScanConfig{StartAddress: 65400, EndAddress: 65535}, ranges: []coreRange{{0, 65535}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Algo = "linear"
			tasks, covered := coreSimulate(t, scanSettings{ScanConfig: tc.cfg}, coreReadableIn(tc.ranges...))
			assert.Equal(t, coreWantCovered(tc.cfg, tc.ranges), covered)
			for _, task := range tasks {
				assert.GreaterOrEqual(t, task.Start, tc.cfg.StartAddress)
				assert.LessOrEqual(t, int(task.Start)+int(task.Count)-1, int(tc.cfg.EndAddress))
			}
		})
	}
}

// Boundary captures the block containing the seed and nothing outside it.
func TestCoreBoundaryStrategy_blockAroundSeed(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.ScanConfig
		ranges []coreRange
		want   []coreRange
	}{
		{
			name:   "block inside the range",
			cfg:    config.ScanConfig{StartAddress: 0, EndAddress: 399, SeedStart: 205, SeedCount: 5},
			ranges: []coreRange{{0, 9}, {200, 329}}, want: []coreRange{{200, 329}},
		},
		{
			name:   "block cut by the range on both sides",
			cfg:    config.ScanConfig{StartAddress: 100, EndAddress: 150, SeedStart: 120, SeedCount: 10},
			ranges: []coreRange{{0, 1000}}, want: []coreRange{{100, 150}},
		},
		{
			name:   "seed is the whole range",
			cfg:    config.ScanConfig{StartAddress: 10, EndAddress: 19, SeedStart: 10, SeedCount: 10},
			ranges: []coreRange{{0, 1000}}, want: []coreRange{{10, 19}},
		},
		{
			name:   "seed is the whole block",
			cfg:    config.ScanConfig{StartAddress: 0, EndAddress: 1000, SeedStart: 500, SeedCount: 3},
			ranges: []coreRange{{500, 502}}, want: []coreRange{{500, 502}},
		},
		{
			name:   "block from address 0",
			cfg:    config.ScanConfig{StartAddress: 0, EndAddress: 300, SeedStart: 5, SeedCount: 2},
			ranges: []coreRange{{0, 250}}, want: []coreRange{{0, 250}},
		},
		{
			name:   "single register seed, single register block",
			cfg:    config.ScanConfig{StartAddress: 0, EndAddress: 100, SeedStart: 50, SeedCount: 1},
			ranges: []coreRange{{50, 50}}, want: []coreRange{{50, 50}},
		},
		{
			name:   "seed not readable",
			cfg:    config.ScanConfig{StartAddress: 0, EndAddress: 100, SeedStart: 50, SeedCount: 4},
			ranges: []coreRange{{50, 52}}, want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Algo = "boundary"
			tasks, covered := coreSimulate(t, scanSettings{ScanConfig: tc.cfg}, coreReadableIn(tc.ranges...))
			assert.Equal(t, coreAddrSet(tc.want...), covered)
			require.NotEmpty(t, tasks)
			assert.Equal(t, ScanTask{Start: tc.cfg.SeedStart, Count: tc.cfg.SeedCount}, tasks[0], "the seed is read first")
			for _, task := range tasks {
				assert.GreaterOrEqual(t, task.Start, tc.cfg.StartAddress)
				assert.LessOrEqual(t, int(task.Start)+int(task.Count)-1, int(tc.cfg.EndAddress))
			}
		})
	}

	// A seed the configuration validation would have rejected is not read at all.
	invalid := []config.ScanConfig{
		{StartAddress: 0, EndAddress: 100, SeedStart: 10, SeedCount: 0},
		{StartAddress: 0, EndAddress: 1000, SeedStart: 10, SeedCount: 126},
		{StartAddress: 50, EndAddress: 100, SeedStart: 49, SeedCount: 5},
		{StartAddress: 0, EndAddress: 100, SeedStart: 99, SeedCount: 3},
	}
	for _, cfg := range invalid {
		cfg.Algo = "boundary"
		tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{0, 65535}))
		assert.Empty(t, tasks, "seed %d+%d in [%d,%d]", cfg.SeedStart, cfg.SeedCount, cfg.StartAddress, cfg.EndAddress)
	}
}

func TestCoreSteppedStrategy(t *testing.T) {
	// coreStepAnchors returns the addresses probed with a single-register read.
	anchors := func(tasks []ScanTask) map[uint16]bool {
		out := map[uint16]bool{}
		for _, task := range tasks {
			if task.Count == 1 {
				out[task.Start] = true
			}
		}
		return out
	}

	t.Run("probes around every step", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 1000, EndAddress: 1300, Step: 100}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn())
		assert.Empty(t, covered)
		// Around every step: the register before, the step itself and the one after, each
		// as a one- and a two-register read. The read past the end of the range is left out.
		assert.Len(t, tasks, 4*6-1)
		got := anchors(tasks)
		for _, step := range []uint16{1000, 1100, 1200, 1300} {
			assert.True(t, got[step-1] && got[step] && got[step+1], "step %d", step)
		}
		assert.NotContains(t, tasks, ScanTask{Start: 1301, Count: 2})
	})

	t.Run("half offset doubles the positions", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 250, Step: 100, StepHalfOffset: true}
		tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn())
		got := anchors(tasks)
		for _, pos := range []uint16{0, 50, 100, 150, 200, 250} {
			assert.True(t, got[pos], "position %d", pos)
		}
		assert.False(t, got[25])
		// Address 0 has no register before it.
		assert.Equal(t, ScanTask{Start: 0, Count: 1}, tasks[0])
	})

	t.Run("a hit is grown to the largest block that reads", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 999, Step: 500}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{500, 539}))
		// 125 and 64 registers do not read; 32 do.
		assert.Equal(t, coreAddrSet(coreRange{500, 531}), covered)
		assert.Contains(t, tasks, ScanTask{Start: 500, Count: 125})
		assert.Contains(t, tasks, ScanTask{Start: 500, Count: 64})
		assert.Contains(t, tasks, ScanTask{Start: 500, Count: 32})
		assert.NotContains(t, tasks, ScanTask{Start: 500, Count: 16}, "expansion stops at the first block that reads")
	})

	t.Run("a hit that cannot be grown", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 999, Step: 500}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{500, 501}))
		assert.Equal(t, coreAddrSet(coreRange{500, 501}), covered)
		for _, size := range steppedExpandSizes {
			assert.Contains(t, tasks, ScanTask{Start: 500, Count: size})
		}
	})

	t.Run("expansion never leaves the range", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 9, Step: 5}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{0, 65535}))
		// From the hit at 0 only 8 and 4 registers fit; from the hit at 4 only 4 fit.
		assert.Contains(t, tasks, ScanTask{Start: 0, Count: 8})
		assert.Contains(t, tasks, ScanTask{Start: 4, Count: 4})
		for _, task := range tasks {
			if task.Count > 2 {
				assert.LessOrEqual(t, int(task.Start)+int(task.Count)-1, 9)
			}
		}
		assert.True(t, covered[0] && covered[7])
	})

	t.Run("range too small for any expansion", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 2, Step: 1}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{0, 65535}))
		assert.Equal(t, coreAddrSet(coreRange{0, 3}), covered, "only the probes read anything")
		for _, task := range tasks {
			assert.LessOrEqual(t, task.Count, uint16(2))
		}
	})

	t.Run("steps stop at the top of the address space", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 63000, EndAddress: 65535, Step: 1000}
		tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn())
		got := anchors(tasks)
		assert.True(t, got[63000] && got[64000] && got[65000])
		for _, task := range tasks {
			assert.GreaterOrEqual(t, task.Start, uint16(62999), "no step wraps around to low addresses")
		}
	})

	t.Run("last step on the last address", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 65535 - 10, EndAddress: 65535, Step: 10}
		tasks, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{65535, 65535}))
		assert.Contains(t, tasks, ScanTask{Start: 65535, Count: 1})
		assert.Equal(t, coreAddrSet(coreRange{65535, 65535}), covered)
	})

	t.Run("step 0 does nothing", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "stepped", StartAddress: 0, EndAddress: 100}
		tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{0, 100}))
		assert.Empty(t, tasks)
	})
}

func TestCoreSunSpecStrategy_invalidBasesPanic(t *testing.T) {
	cfg := config.ScanConfig{Algo: "sunspec", SunSpecBases: "40000,not-a-number"}
	strategy, err := newScanStrategy(scanSettings{ScanConfig: cfg})
	require.NoError(t, err)
	assert.PanicsWithValue(t, "modbusctl: invalid SunSpecBases after validation: "+coreSunSpecBasesError(t, cfg.SunSpecBases), func() {
		strategy.Init(scanSettings{ScanConfig: cfg})
	})
}

// coreSunSpecBasesError returns the parse error text for an invalid base list.
func coreSunSpecBasesError(t *testing.T, bases string) string {
	t.Helper()
	_, err := config.ParseSunSpecBases(bases)
	require.Error(t, err)
	return err.Error()
}

// Simulated SunSpec walks that the in-memory device tests do not reach.
func TestCoreSunSpecStrategy_walkEnds(t *testing.T) {
	header := func(id, length uint16) []byte {
		return []byte{byte(id >> 8), byte(id), byte(length >> 8), byte(length)}
	}
	start := func(t *testing.T, cfg config.ScanConfig) ScanStrategy {
		t.Helper()
		cfg.Algo = "sunspec"
		s, err := newScanStrategy(scanSettings{ScanConfig: cfg})
		require.NoError(t, err)
		s.Init(scanSettings{ScanConfig: cfg})
		return s
	}
	next := func(t *testing.T, s ScanStrategy) ScanTask {
		t.Helper()
		task, ok := s.Next()
		require.True(t, ok)
		return task
	}

	t.Run("model with length 0 is malformed", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBase: 40000})
		task := next(t, s)
		assert.Equal(t, ScanTask{Start: 40002, Count: 2}, task)
		s.OnResult(task, ScanResult{Success: true, Data: header(101, 0)})
		assert.True(t, s.Done())
		_, ok := s.Next()
		assert.False(t, ok)
	})

	t.Run("model running past the address space", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBase: 65000})
		task := next(t, s)
		s.OnResult(task, ScanResult{Success: true, Data: header(101, 600)})
		assert.True(t, s.Done())
	})

	t.Run("short header read", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBase: 40000})
		task := next(t, s)
		s.OnResult(task, ScanResult{Success: true, Data: []byte{0x00, 0x65}})
		assert.True(t, s.Done())
	})

	t.Run("body read fails", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBase: 40000})
		task := next(t, s)
		s.OnResult(task, ScanResult{Success: true, Data: header(101, 10)})
		body := next(t, s)
		assert.Equal(t, ScanTask{Start: 40004, Count: 10}, body)
		s.OnResult(body, ScanResult{Success: false})
		assert.True(t, s.Done())
		// A result delivered after the end changes nothing.
		s.OnResult(body, ScanResult{Success: true, Data: header(1, 2)})
		assert.True(t, s.Done())
	})

	t.Run("marker at a later candidate base", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBases: "0,100"})
		first := next(t, s)
		assert.Equal(t, ScanTask{Start: 0, Count: 2}, first)
		// Readable, but not the marker.
		s.OnResult(first, ScanResult{Success: true, Data: []byte{0x00, 0x01, 0x00, 0x02}})
		second := next(t, s)
		assert.Equal(t, ScanTask{Start: 100, Count: 2}, second)
		s.OnResult(second, ScanResult{Success: true, Data: []byte{0x53, 0x75, 0x6E, 0x53}})
		assert.False(t, s.Done())
		assert.Equal(t, ScanTask{Start: 102, Count: 2}, next(t, s))
	})

	t.Run("no candidate base has the marker", func(t *testing.T) {
		s := start(t, config.ScanConfig{SunSpecBases: "0,100"})
		s.OnResult(next(t, s), ScanResult{Success: false})
		s.OnResult(next(t, s), ScanResult{Success: true, Data: []byte{0x53, 0x75}})
		assert.True(t, s.Done())
	})
}

func TestCoreNewScanStrategy_names(t *testing.T) {
	for _, algo := range config.ScanAlgorithms() {
		s, err := newScanStrategy(scanSettings{ScanConfig: config.ScanConfig{Algo: algo}})
		require.NoError(t, err)
		assert.Equal(t, algo, s.Name())
	}
	s, err := newScanStrategy(scanSettings{})
	require.NoError(t, err)
	assert.Equal(t, "safe", s.Name(), "no algorithm means safe")
}

func TestCoreScanDebugf(t *testing.T) {
	scanDebugf(nil, "ignored %d", 1) // A nil settings pointer is tolerated.
	assert.Equal(t, []uint16(nil), candidateCountsSafe(10, 9))
	assert.Equal(t, uint16(5), clampAddr(5, 1, 9))
	assert.Equal(t, uint16(1), clampAddr(0, 1, 9))
	assert.Equal(t, uint16(9), clampAddr(10, 1, 9))
}

// The result of a safe-scan left-boundary probe must not move the scan position: handling it
// like a ladder read used to drop one register per block.
func TestCoreSafe_leftBoundaryProbeDoesNotSkipRegisters(t *testing.T) {
	cfg := config.ScanConfig{Algo: "safe", StartAddress: 0, EndAddress: 40}
	ranges := []coreRange{{10, 40}}
	_, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(ranges...))
	assert.Equal(t, coreWantCovered(cfg, ranges), covered)
}

// The safe-scan left-boundary probe must stay inside the requested range.
func TestCoreSafe_neverReadsBelowRangeStart(t *testing.T) {
	cfg := config.ScanConfig{Algo: "safe", StartAddress: 1000, EndAddress: 1999}
	tasks, _ := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{1000, 1000}))
	for _, task := range tasks {
		assert.GreaterOrEqual(t, task.Start, cfg.StartAddress)
	}
}

// Scans that end at 65535 (the default --end) must terminate and capture the last registers:
// 16-bit position arithmetic used to wrap around instead of passing the end of the range.
func TestCoreScan_terminatesAtTopOfAddressSpace(t *testing.T) {
	for _, algo := range []string{"safe", "smart", "deep"} {
		for name, ranges := range map[string][]coreRange{
			"nothing readable":    nil,
			"readable to the top": {{65500, 65535}},
			"gap before the top":  {{65400, 65450}, {65530, 65535}},
		} {
			t.Run(algo+"/"+name, func(t *testing.T) {
				cfg := config.ScanConfig{Algo: algo, StartAddress: 65300, EndAddress: 65535}
				_, covered := coreSimulate(t, scanSettings{ScanConfig: cfg}, coreReadableIn(ranges...))
				assert.Equal(t, coreWantCovered(cfg, ranges), covered)
			})
		}
	}
}

// A linear scan must finish when its last probe fails near address 65535: a 16-bit probe
// position used to wrap around and restart the scan from a low address forever.
func TestCoreScanRegression_linearNeverEndsAtTopOfAddressSpace(t *testing.T) {
	cfg := config.ScanConfig{Algo: "linear", StartAddress: 65000, EndAddress: 65535}
	tasks, _, finished := coreRunStrategy(t, scanSettings{ScanConfig: cfg}, coreReadableIn())
	require.True(t, finished, "still running after %d reads", len(tasks))
	for _, task := range tasks {
		assert.GreaterOrEqual(t, task.Start, cfg.StartAddress)
	}
}

// A linear scan of the full range 0-65535 must not issue zero-register reads: the size of
// the first probe used to overflow to 0.
func TestCoreScanRegression_linearZeroCountReadOnFullRange(t *testing.T) {
	cfg := config.ScanConfig{Algo: "linear", StartAddress: 0, EndAddress: 65535}
	strategy, err := newScanStrategy(scanSettings{ScanConfig: cfg})
	require.NoError(t, err)
	strategy.Init(scanSettings{ScanConfig: cfg})
	task, ok := strategy.Next()
	require.True(t, ok)
	assert.Equal(t, ScanTask{Start: 0, Count: 125}, task)
}

// The boundary scan must find the left edge of a block of any size with reads of at most
// 125 registers: the left binary search used to issue oversized reads and to give up after
// the first failed read.
func TestCoreScanRegression_boundaryLeftSearch(t *testing.T) {
	t.Run("edge beyond the expansion steps", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "boundary", StartAddress: 0, EndAddress: 1100, SeedStart: 1000, SeedCount: 10}
		tasks, _, _ := coreRunStrategy(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{200, 1050}))
		for _, task := range tasks {
			assert.LessOrEqual(t, task.Count, uint16(MaxBlockSize), "read of %d registers at %d", task.Count, task.Start)
		}
	})
	t.Run("edge inside the last expansion step", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "boundary", StartAddress: 0, EndAddress: 1100, SeedStart: 1000, SeedCount: 10}
		_, covered, _ := coreRunStrategy(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{800, 1050}))
		assert.True(t, covered[800], "the block starts at 800")
	})
}

// The boundary scan must handle a block that reaches address 65535: its exclusive end used
// to wrap around to 0.
func TestCoreScanRegression_boundaryAtTopOfAddressSpace(t *testing.T) {
	t.Run("seed ending at 65535 is read", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "boundary", StartAddress: 65000, EndAddress: 65535, SeedStart: 65530, SeedCount: 6}
		tasks, _, _ := coreRunStrategy(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{65400, 65535}))
		assert.NotEmpty(t, tasks)
	})
	t.Run("right search stays in range", func(t *testing.T) {
		cfg := config.ScanConfig{Algo: "boundary", StartAddress: 65000, EndAddress: 65535, SeedStart: 65500, SeedCount: 10}
		tasks, _, finished := coreRunStrategy(t, scanSettings{ScanConfig: cfg}, coreReadableIn(coreRange{65400, 65535}))
		assert.True(t, finished, "still running after %d reads", len(tasks))
		for _, task := range tasks[:min(len(tasks), 100)] {
			assert.GreaterOrEqual(t, task.Start, cfg.StartAddress, "read of %d registers at %d", task.Count, task.Start)
			assert.LessOrEqual(t, task.Count, uint16(MaxBlockSize), "read of %d registers at %d", task.Count, task.Start)
		}
	})
}

// SPDX-License-Identifier: MIT

package modbus

import (
	"container/heap"
	"fmt"
	"io"
	"sort"

	"github.com/otfabric/go-modbus/sunspec"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
)

// scanSettings bundles config.ScanConfig (flags/env only) with execution-only debug output.
// ScanAndRecord builds ss := scanSettings{ScanConfig: cfg} and sets DebugWriter when cfg.Debug.
type scanSettings struct {
	config.ScanConfig
	DebugWriter io.Writer `json:"-"`
}

func scanDebugf(s *scanSettings, format string, args ...any) {
	if s == nil || !s.Debug {
		return
	}
	w := s.DebugWriter
	if w == nil {
		w = io.Discard
	}
	_, _ = fmt.Fprintf(w, format, args...)
}

// --- Scan strategy types and interface ---

// ScanTask is a single read request (start address and count).
type ScanTask struct {
	Start uint16
	Count uint16
}

// ScanOutcomeType classifies the result of a read for strategy decisions.
type ScanOutcomeType string

const (
	ScanOutcomeSuccess   ScanOutcomeType = "success"
	ScanOutcomeException ScanOutcomeType = "exception" // Modbus exception (e.g. Illegal Data Address)
	ScanOutcomeTimeout   ScanOutcomeType = "timeout"
	ScanOutcomeTransport ScanOutcomeType = "transport_error"
	ScanOutcomeProtocol  ScanOutcomeType = "protocol_error"
	ScanOutcomeUnknown   ScanOutcomeType = "unknown"
)

// ScanResult holds the outcome of executing a ScanTask.
type ScanResult struct {
	Success           bool
	Start             uint16
	Count             uint16
	Data              []byte
	RequestTimestamp  int64
	ResponseTimestamp int64
	Err               error
	OutcomeType       ScanOutcomeType // classification for strategy branching
	ExceptionCode     uint8           // Modbus exception code when OutcomeType == ScanOutcomeException; 0 otherwise
	RTTNanos          int64           // response timestamp − request timestamp
}

// Interval represents a contiguous address range (used by smart/deep strategies).
type Interval struct {
	Start uint16
	Count uint16
}

// ScanStats holds aggregate scan statistics.
type ScanStats struct {
	TotalRequests       int
	SuccessCount        int
	FailCount           int
	ExceptionCount      int // Modbus exception (e.g. Illegal Data Address)
	TimeoutCount        int // request timed out
	TransportErrorCount int // connection/transport errors
	BlocksCaptured      int
	RegistersCaptured   int
	TotalDurationNanos  int64
	ResponseTimeNanos   int64 // sum of (response - request) for computing average
}

// ScanStrategy decides the next read(s) and how to react to results.
type ScanStrategy interface {
	Name() string
	Init(cfg scanSettings)
	Next() (ScanTask, bool)
	OnResult(task ScanTask, result ScanResult)
	Done() bool
}

// newScanStrategy returns the strategy for the given config's algo (normalized to lowercase).
func newScanStrategy(cfg scanSettings) (ScanStrategy, error) {
	algo := string(config.ScanAlgorithmForExecution(&cfg.ScanConfig))
	switch algo {
	case "safe":
		return newSafeStrategy(cfg), nil
	case "smart":
		return newSmartStrategy(cfg), nil
	case "deep":
		return newDeepStrategy(cfg), nil
	case "stepped":
		return newSteppedStrategy(cfg), nil
	case "linear":
		return newLinearStrategy(cfg), nil
	case "boundary":
		return newBoundaryStrategy(cfg), nil
	case "sunspec":
		return newSunSpecStrategy(cfg), nil
	default:
		return nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("unsupported scan algorithm %q", cfg.Algo), nil)
	}
}

// --- Safe strategy (conservative linear probing with descending block sizes) ---

var safeCandidateSizes = []uint16{125, 64, 32, 16, 8, 4, 2, 1}

func candidateCountsSafe(start, end uint16) []uint16 {
	if start > end {
		return nil
	}
	remaining := end - start + 1
	var out []uint16
	for _, size := range safeCandidateSizes {
		if size <= remaining {
			out = append(out, size)
		}
	}
	return out
}

type safeStrategy struct {
	cfg scanSettings
	// current is wider than an address so that it can step past end when end is 65535.
	current                  uint32
	end                      uint16
	done                     bool
	candidates               []uint16
	candidateIndex           int
	hadFailureThisAddr       bool   // true if we had at least one failure at current address (Milestone D)
	leftBoundaryProbeAddr    uint16 // address to probe (0 is valid); use with leftBoundaryProbePending
	leftBoundaryProbePending bool
	// leftBoundaryProbeInFlight marks the task handed out last as the left-boundary probe: its
	// result is only captured by the executor and must not move the scan position.
	leftBoundaryProbeInFlight bool
}

func newSafeStrategy(cfg scanSettings) *safeStrategy {
	return &safeStrategy{cfg: cfg}
}

func (s *safeStrategy) Name() string { return "safe" }

func (s *safeStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.current = uint32(cfg.StartAddress)
	s.end = cfg.EndAddress
	s.done = false
	s.candidates = nil
	s.candidateIndex = 0
	s.hadFailureThisAddr = false
	s.leftBoundaryProbePending = false
	s.leftBoundaryProbeInFlight = false
	scanDebugf(&s.cfg, "DEBUG [safe] init: range [%d,%d]\n", s.current, s.end)
}

func (s *safeStrategy) Next() (ScanTask, bool) {
	// Milestone D: one left-boundary probe when we succeeded after failure (ladder boundary)
	if s.leftBoundaryProbePending {
		s.leftBoundaryProbePending = false
		s.leftBoundaryProbeInFlight = true
		scanDebugf(&s.cfg, "DEBUG [safe] left-boundary probe: addr=%d count=1\n", s.leftBoundaryProbeAddr)
		return ScanTask{Start: s.leftBoundaryProbeAddr, Count: 1}, true
	}
	if s.done || s.current > uint32(s.end) {
		if s.current > uint32(s.end) {
			scanDebugf(&s.cfg, "DEBUG [safe] done: current=%d > end=%d\n", s.current, s.end)
		}
		return ScanTask{}, false
	}
	if s.candidates == nil || s.candidateIndex >= len(s.candidates) {
		s.candidates = candidateCountsSafe(uint16(s.current), s.end)
		s.candidateIndex = 0
		if len(s.candidates) == 0 {
			s.done = true
			scanDebugf(&s.cfg, "DEBUG [safe] no candidates at current=%d, done\n", s.current)
			return ScanTask{}, false
		}
		scanDebugf(&s.cfg, "DEBUG [safe] candidates at current=%d: %v\n", s.current, s.candidates)
	}
	count := s.candidates[s.candidateIndex]
	s.candidateIndex++
	return ScanTask{Start: uint16(s.current), Count: count}, true
}

func (s *safeStrategy) OnResult(task ScanTask, result ScanResult) {
	if s.leftBoundaryProbeInFlight {
		s.leftBoundaryProbeInFlight = false
		return
	}
	if result.Success {
		// Probe the register just below a block found after failures, but never below the range.
		if s.hadFailureThisAddr && task.Start > s.cfg.StartAddress {
			s.leftBoundaryProbeAddr = task.Start - 1
			s.leftBoundaryProbePending = true
			scanDebugf(&s.cfg, "DEBUG [safe] scheduled left-boundary probe at addr=%d (after success at %d)\n", s.leftBoundaryProbeAddr, task.Start)
		}
		prev := s.current
		s.current += uint32(task.Count)
		s.candidates = nil
		s.candidateIndex = 0
		s.hadFailureThisAddr = false
		scanDebugf(&s.cfg, "DEBUG [safe] success: advance current %d -> %d (count=%d)\n", prev, s.current, task.Count)
	} else {
		s.hadFailureThisAddr = true
		// Exhausted all candidates for this address; move to next
		if s.candidateIndex >= len(s.candidates) {
			prev := s.current
			s.current++
			s.candidates = nil
			s.candidateIndex = 0
			scanDebugf(&s.cfg, "DEBUG [safe] full failure at addr %d, advance current %d -> %d\n", prev, prev, s.current)
		}
	}
}

func (s *safeStrategy) Done() bool {
	// Not done while a left-boundary probe is pending (must be returned before completion).
	if s.leftBoundaryProbePending {
		return false
	}
	return s.current > uint32(s.end)
}

// --- Smart strategy (priority queue by interval size, split-on-fail) ---

// smartQueue is a min-heap by Count (smaller intervals preferred for boundary discovery).
type smartQueue []Interval

func (q smartQueue) Len() int { return len(q) }
func (q smartQueue) Less(i, j int) bool {
	if q[i].Count != q[j].Count {
		return q[i].Count < q[j].Count
	}
	return q[i].Start < q[j].Start
}
func (q smartQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *smartQueue) Push(x any)   { *q = append(*q, x.(Interval)) }
func (q *smartQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[0 : n-1]
	return item
}

type smartStrategy struct {
	cfg     scanSettings
	queue   smartQueue
	visited map[uint32]bool // key: start<<16|count
}

func newSmartStrategy(cfg scanSettings) *smartStrategy {
	return &smartStrategy{cfg: cfg, visited: make(map[uint32]bool)}
}

func (s *smartStrategy) Name() string { return "smart" }

func (s *smartStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.visited = make(map[uint32]bool)
	s.queue = nil
	// 32-bit arithmetic: with end at 65535 a 16-bit start would wrap and never pass it.
	end := uint32(cfg.EndAddress)
	for start := uint32(cfg.StartAddress); start <= end; {
		count := end - start + 1
		if count > MaxBlockSize {
			count = MaxBlockSize
		}
		heap.Push(&s.queue, Interval{Start: uint16(start), Count: uint16(count)})
		start += count
	}
	scanDebugf(&s.cfg, "DEBUG [smart] init: range [%d,%d] initial intervals=%d\n", cfg.StartAddress, cfg.EndAddress, s.queue.Len())
}

func intervalKey(start, count uint16) uint32 {
	return uint32(start)<<16 | uint32(count)
}

func (s *smartStrategy) Next() (ScanTask, bool) {
	for s.queue.Len() > 0 {
		iv := heap.Pop(&s.queue).(Interval)
		key := intervalKey(iv.Start, iv.Count)
		if s.visited[key] {
			continue
		}
		s.visited[key] = true
		scanDebugf(&s.cfg, "DEBUG [smart] pop: start=%d count=%d (queue remaining=%d)\n", iv.Start, iv.Count, s.queue.Len())
		return ScanTask(iv), true
	}
	scanDebugf(&s.cfg, "DEBUG [smart] queue empty, done\n")
	return ScanTask{}, false
}

func (s *smartStrategy) OnResult(task ScanTask, result ScanResult) {
	if result.Success {
		scanDebugf(&s.cfg, "DEBUG [smart] success: [%d,%d] recorded\n", task.Start, task.Start+task.Count-1)
		return // interval recorded as readable (recorded by the executor)
	}
	if task.Count > 1 {
		leftCount := task.Count / 2
		rightStart := task.Start + leftCount
		rightCount := task.Count - leftCount
		heap.Push(&s.queue, Interval{Start: task.Start, Count: leftCount})
		heap.Push(&s.queue, Interval{Start: rightStart, Count: rightCount})
		scanDebugf(&s.cfg, "DEBUG [smart] split fail: [%d,%d] -> left [%d,%d] right [%d,%d] (queue size=%d)\n",
			task.Start, task.Start+task.Count-1, task.Start, task.Start+leftCount-1, rightStart, rightStart+rightCount-1, s.queue.Len())
	} else {
		scanDebugf(&s.cfg, "DEBUG [smart] fail singleton: addr=%d unreadable\n", task.Start)
	}
}

func (s *smartStrategy) Done() bool {
	return len(s.queue) == 0
}

// --- Deep strategy (smart + evidence-driven boundary refinement) ---

const (
	deepRefinementWindow = 8 // smaller window at boundaries only
	deepRefinementCap    = 500
)

var deepRefinementCounts = []uint16{1, 2, 4, 8}

type deepStrategy struct {
	cfg               scanSettings
	phase             int // 1 = smart, 2 = refinement
	smart             *smartStrategy
	readableIntervals []Interval
	failedIntervals   []Interval // phase 1 failures for boundary evidence
	refinementQueue   []ScanTask
	refinementSeen    map[uint32]bool // dedup (start<<16|count)
}

func newDeepStrategy(cfg scanSettings) *deepStrategy {
	return &deepStrategy{cfg: cfg, refinementSeen: make(map[uint32]bool)}
}

func (s *deepStrategy) Name() string { return "deep" }

func (s *deepStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.phase = 1
	s.smart = newSmartStrategy(cfg)
	s.smart.Init(cfg)
	s.readableIntervals = nil
	s.failedIntervals = nil
	s.refinementQueue = nil
	s.refinementSeen = make(map[uint32]bool)
	scanDebugf(&s.cfg, "DEBUG [deep] init: phase 1 (smart), range [%d,%d]\n", cfg.StartAddress, cfg.EndAddress)
}

func (s *deepStrategy) Next() (ScanTask, bool) {
	if s.phase == 1 {
		task, ok := s.smart.Next()
		if ok {
			return task, true
		}
		// Smart phase done; build refinement queue from readable intervals
		scanDebugf(&s.cfg, "DEBUG [deep] phase 1 done: readable=%d failed=%d, building refinement\n", len(s.readableIntervals), len(s.failedIntervals))
		s.phase = 2
		s.buildRefinementQueue()
		scanDebugf(&s.cfg, "DEBUG [deep] phase 2: refinement queue size=%d\n", len(s.refinementQueue))
	}
	// Phase 2: serve from refinement queue
	if len(s.refinementQueue) > 0 {
		task := s.refinementQueue[0]
		s.refinementQueue = s.refinementQueue[1:]
		scanDebugf(&s.cfg, "DEBUG [deep] refinement task: start=%d count=%d (remaining=%d)\n", task.Start, task.Count, len(s.refinementQueue))
		return task, true
	}
	scanDebugf(&s.cfg, "DEBUG [deep] phase 2 done\n")
	return ScanTask{}, false
}

// hasFailureAdjacent returns true if there is a failed interval touching the given address
// (either containing addr-1 for a left boundary or containing addr for a right boundary).
func (s *deepStrategy) hasFailureAdjacent(addr uint16, leftBound bool) bool {
	for _, f := range s.failedIntervals {
		fEnd := f.Start + f.Count - 1
		if leftBound {
			// left boundary: failure touches (addr-1) or addr
			if addr > 0 && (f.Start <= addr-1 && fEnd >= addr-1) || (f.Start <= addr && fEnd >= addr) {
				return true
			}
		} else {
			// right boundary: failure touches (addr) or (addr+1)
			if (f.Start <= addr && fEnd >= addr) || (addr < 65535 && f.Start <= addr+1 && fEnd >= addr+1) {
				return true
			}
		}
	}
	return false
}

func (s *deepStrategy) buildRefinementQueue() {
	globalStart := s.cfg.StartAddress
	globalEnd := s.cfg.EndAddress
	for _, iv := range s.readableIntervals {
		if len(s.refinementQueue) >= deepRefinementCap {
			break
		}
		intervalStart := iv.Start
		intervalEnd := iv.Start + iv.Count - 1
		// Left boundary: refine only if there is evidence (failure adjacent to start)
		if intervalStart > globalStart && s.hasFailureAdjacent(intervalStart, true) {
			leftStart := int32(intervalStart) - deepRefinementWindow
			if leftStart < int32(globalStart) {
				leftStart = int32(globalStart)
			}
			leftEnd := int32(intervalStart) + deepRefinementWindow
			if leftEnd > int32(globalEnd) {
				leftEnd = int32(globalEnd)
			}
			for start := uint16(leftStart); start <= uint16(leftEnd) && len(s.refinementQueue) < deepRefinementCap; start++ {
				for _, count := range deepRefinementCounts {
					if int32(start)+int32(count)-1 <= int32(globalEnd) {
						key := intervalKey(start, count)
						if !s.refinementSeen[key] {
							s.refinementSeen[key] = true
							s.refinementQueue = append(s.refinementQueue, ScanTask{Start: start, Count: count})
						}
					}
				}
			}
		}
		// Right boundary: refine only if there is evidence (failure adjacent to end)
		if intervalEnd < globalEnd && s.hasFailureAdjacent(intervalEnd, false) {
			rightStart := int32(intervalEnd) - deepRefinementWindow
			if rightStart < int32(globalStart) {
				rightStart = int32(globalStart)
			}
			rightEnd := int32(intervalEnd) + deepRefinementWindow
			if rightEnd > int32(globalEnd) {
				rightEnd = int32(globalEnd)
			}
			for start := uint16(rightStart); start <= uint16(rightEnd) && len(s.refinementQueue) < deepRefinementCap; start++ {
				for _, count := range deepRefinementCounts {
					if int32(start)+int32(count)-1 <= int32(globalEnd) {
						key := intervalKey(start, count)
						if !s.refinementSeen[key] {
							s.refinementSeen[key] = true
							s.refinementQueue = append(s.refinementQueue, ScanTask{Start: start, Count: count})
						}
					}
				}
			}
		}
	}
}

func (s *deepStrategy) OnResult(task ScanTask, result ScanResult) {
	if s.phase == 1 {
		s.smart.OnResult(task, result)
		if result.Success {
			s.readableIntervals = append(s.readableIntervals, Interval(task))
			scanDebugf(&s.cfg, "DEBUG [deep] phase 1: readable + [%d,%d] (total=%d)\n", task.Start, task.Start+task.Count-1, len(s.readableIntervals))
		} else {
			s.failedIntervals = append(s.failedIntervals, Interval(task))
			scanDebugf(&s.cfg, "DEBUG [deep] phase 1: failed + [%d,%d] (total=%d)\n", task.Start, task.Start+task.Count-1, len(s.failedIntervals))
		}
	}
	// Phase 2: no further splitting, executor already recorded the read on success
}

func (s *deepStrategy) Done() bool {
	if s.phase == 1 {
		return false // not done until phase 2 is built and drained
	}
	return len(s.refinementQueue) == 0
}

// --- Stepped strategy (quick probe at step positions, then expand on hit) ---

var steppedExpandSizes = []uint16{125, 64, 32, 16, 8, 4}

type steppedStrategy struct {
	cfg           scanSettings
	stepPositions []uint16
	stepIndex     int
	probeTasks    []ScanTask
	probeIndex    int
	hasHit        bool // true when a probe succeeded at this step (address 0 is valid)
	hitAddr       uint16
	expandIndex   int // -1 = not expanding, 0..5 = current expansion size index
}

func newSteppedStrategy(cfg scanSettings) *steppedStrategy {
	return &steppedStrategy{cfg: cfg, expandIndex: -1}
}

func (s *steppedStrategy) Name() string { return "stepped" }

func (s *steppedStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.stepPositions = nil
	// Step == 0: strategy done immediately (no tasks); spec 5.1 / 5.4
	if cfg.Step == 0 {
		return
	}
	start := cfg.StartAddress
	end := cfg.EndAddress
	step := cfg.Step
	if step < 1 {
		step = 1000
	}
	// Build positions: base steps and optionally step/2 offsets (Milestone C)
	seen := make(map[uint16]bool)
	add := func(p uint16) {
		if p <= end && !seen[p] {
			seen[p] = true
			s.stepPositions = append(s.stepPositions, p)
		}
	}
	for pos := start; pos <= end; {
		add(pos)
		if cfg.StepHalfOffset && step >= 2 {
			half := pos + step/2
			add(half)
		}
		if pos >= end {
			break
		}
		prev := pos
		pos += step
		if pos < prev {
			break
		}
	}
	sort.Slice(s.stepPositions, func(i, j int) bool { return s.stepPositions[i] < s.stepPositions[j] })
	s.stepIndex = 0
	s.probeTasks = nil
	s.probeIndex = 0
	s.hasHit = false
	s.hitAddr = 0
	s.expandIndex = -1
	s.buildProbeTasks()
	scanDebugf(&s.cfg, "DEBUG [stepped] init: range [%d,%d] step=%d halfOffset=%v positions=%d\n", start, end, step, cfg.StepHalfOffset, len(s.stepPositions))
}

func clampAddr(addr uint16, min, max uint16) uint16 {
	if addr < min {
		return min
	}
	if addr > max {
		return max
	}
	return addr
}

func (s *steppedStrategy) buildProbeTasks() {
	s.probeTasks = nil
	s.probeIndex = 0
	if s.stepIndex >= len(s.stepPositions) {
		return
	}
	pos := s.stepPositions[s.stepIndex]
	start := s.cfg.StartAddress
	end := s.cfg.EndAddress
	minAddr := uint16(0)
	if start > 0 {
		minAddr = start - 1
	}
	maxAddr := uint16(65535)
	if end < 65535 {
		maxAddr32 := uint32(end) + 1
		if maxAddr32 > 65535 {
			maxAddr32 = 65535
		}
		maxAddr = uint16(maxAddr32)
	}
	var positions []uint16
	if pos > 0 {
		positions = append(positions, clampAddr(pos-1, minAddr, maxAddr))
	}
	positions = append(positions, clampAddr(pos, minAddr, maxAddr))
	if pos < 65535 {
		positions = append(positions, clampAddr(pos+1, minAddr, maxAddr))
	}
	for _, addr := range positions {
		for _, count := range []uint16{1, 2} {
			if uint32(addr)+uint32(count)-1 > 65535 {
				continue
			}
			endAddr := addr + count - 1
			if endAddr > maxAddr {
				continue
			}
			s.probeTasks = append(s.probeTasks, ScanTask{Start: addr, Count: count})
		}
	}
}

// maxExpandEnd returns the maximum inclusive end address for expansion reads (strictly within configured range).
func (s *steppedStrategy) maxExpandEnd() uint16 {
	return s.cfg.EndAddress
}

func (s *steppedStrategy) Next() (ScanTask, bool) {
	if s.stepIndex >= len(s.stepPositions) {
		return ScanTask{}, false
	}
	if s.expandIndex >= 0 && s.expandIndex < len(steppedExpandSizes) {
		maxEnd := s.maxExpandEnd()
		for s.expandIndex < len(steppedExpandSizes) {
			size := steppedExpandSizes[s.expandIndex]
			// Expansion strictly within [StartAddress, EndAddress]; read covers [hitAddr, hitAddr+size-1]
			if uint32(s.hitAddr)+uint32(size)-1 <= uint32(maxEnd) {
				scanDebugf(&s.cfg, "DEBUG [stepped] expansion: stepIndex=%d hitAddr=%d size=%d (expandIndex=%d)\n", s.stepIndex, s.hitAddr, size, s.expandIndex)
				return ScanTask{Start: s.hitAddr, Count: size}, true
			}
			s.expandIndex++
		}
		s.hasHit = false
		s.hitAddr = 0
		s.expandIndex = -1
		s.stepIndex++
		s.buildProbeTasks()
		return s.Next()
	}
	if s.probeIndex < len(s.probeTasks) {
		task := s.probeTasks[s.probeIndex]
		s.probeIndex++
		scanDebugf(&s.cfg, "DEBUG [stepped] probe: stepIndex=%d pos=%d probe %d/%d (start=%d count=%d)\n",
			s.stepIndex, s.stepPositions[s.stepIndex], s.probeIndex, len(s.probeTasks), task.Start, task.Count)
		return task, true
	}
	if s.hasHit {
		s.expandIndex = 0
		maxEnd := s.maxExpandEnd()
		// Find first expansion size that fits within [StartAddress, EndAddress]
		for s.expandIndex < len(steppedExpandSizes) {
			size := steppedExpandSizes[s.expandIndex]
			if uint32(s.hitAddr)+uint32(size)-1 <= uint32(maxEnd) {
				scanDebugf(&s.cfg, "DEBUG [stepped] hit at stepIndex=%d hitAddr=%d, start expansion size=%d\n", s.stepIndex, s.hitAddr, size)
				return ScanTask{Start: s.hitAddr, Count: size}, true
			}
			s.expandIndex++
		}
		// No expansion size fits; advance to next step
		s.hasHit = false
		s.hitAddr = 0
		s.expandIndex = -1
		s.stepIndex++
		s.buildProbeTasks()
		return s.Next()
	}
	s.stepIndex++
	if s.stepIndex >= len(s.stepPositions) {
		scanDebugf(&s.cfg, "DEBUG [stepped] all steps done\n")
		return ScanTask{}, false
	}
	s.hasHit = false
	s.hitAddr = 0
	s.buildProbeTasks()
	scanDebugf(&s.cfg, "DEBUG [stepped] advance to stepIndex=%d pos=%d (%d probes)\n", s.stepIndex, s.stepPositions[s.stepIndex], len(s.probeTasks))
	return s.Next()
}

func (s *steppedStrategy) OnResult(task ScanTask, result ScanResult) {
	if s.expandIndex >= 0 && s.expandIndex < len(steppedExpandSizes) {
		if result.Success {
			scanDebugf(&s.cfg, "DEBUG [stepped] expansion success: [%d,%d], advance to next step\n", task.Start, task.Start+task.Count-1)
			s.hasHit = false
			s.hitAddr = 0
			s.expandIndex = -1
			s.stepIndex++
			s.buildProbeTasks()
		} else {
			s.expandIndex++
			scanDebugf(&s.cfg, "DEBUG [stepped] expansion fail size=%d (expandIndex=%d)\n", task.Count, s.expandIndex)
			if s.expandIndex >= len(steppedExpandSizes) {
				s.hasHit = false
				s.hitAddr = 0
				s.expandIndex = -1
				s.stepIndex++
				s.buildProbeTasks()
			}
		}
		return
	}
	if result.Success && !s.hasHit {
		s.hasHit = true
		s.hitAddr = task.Start
		scanDebugf(&s.cfg, "DEBUG [stepped] probe hit: addr=%d (stepIndex=%d)\n", task.Start, s.stepIndex)
	}
}

func (s *steppedStrategy) Done() bool {
	if s.stepIndex >= len(s.stepPositions) {
		return true
	}
	if s.expandIndex >= 0 && s.expandIndex < len(steppedExpandSizes) {
		return false
	}
	if s.probeIndex < len(s.probeTasks) {
		return false
	}
	if s.hasHit {
		return false
	}
	return s.stepIndex >= len(s.stepPositions)
}

// --- Linear strategy (125-aligned blocks, extend forward, tail/backward via binary search) ---

type linearPhase int

const (
	linearProbe linearPhase = iota
	linearBackward
	linearForward
	linearTail
)

// Positions and counts are 32 bits wide so that probeStart and blockEnd (an exclusive bound)
// can step past address 65535 instead of wrapping around.
type linearStrategy struct {
	cfg                scanSettings
	start              uint32
	end                uint32
	phase              linearPhase
	probeStart         uint32
	hadProbeFailure    bool
	originalBlockStart uint32
	blockStart         uint32
	blockEnd           uint32
	// Backward binary search: find max K such that read(originalBlockStart-K, K) succeeds.
	backwardLow  uint32
	backwardHigh uint32
	backwardBest uint32
	// Tail binary search: find max count C such that read(blockEnd, C) succeeds.
	tailLow             uint32
	tailHigh            uint32
	tailBest            uint32
	done                bool
	hasGapProbe         bool // true when a gap probe is pending (address 0 is valid)
	gapProbeAddr        uint32
	lastTaskWasGapProbe bool // last Next() was gap probe; OnResult must not update probe state
}

func newLinearStrategy(cfg scanSettings) *linearStrategy {
	return &linearStrategy{cfg: cfg}
}

func (s *linearStrategy) Name() string { return "linear" }

func (s *linearStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.start = uint32(cfg.StartAddress)
	s.end = uint32(cfg.EndAddress)
	s.phase = linearProbe
	s.probeStart = uint32(cfg.StartAddress)
	s.hadProbeFailure = false
	s.done = false
	s.blockStart = 0
	s.blockEnd = 0
	s.originalBlockStart = 0
	s.backwardLow = 0
	s.backwardHigh = 0
	s.backwardBest = 0
	s.tailLow = 0
	s.tailHigh = 0
	s.tailBest = 0
	scanDebugf(&s.cfg, "DEBUG [linear] init: range [%d,%d] phase=Probe probeStart=%d\n", s.start, s.end, s.probeStart)
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func (s *linearStrategy) Next() (ScanTask, bool) {
	if s.done {
		return ScanTask{}, false
	}
	switch s.phase {
	case linearProbe:
		// Milestone D: gap probe for possible island between blocks
		if s.hasGapProbe {
			s.hasGapProbe = false
			s.lastTaskWasGapProbe = true
			scanDebugf(&s.cfg, "DEBUG [linear] gap probe: addr=%d (between block and next 125)\n", s.gapProbeAddr)
			return ScanTask{Start: uint16(s.gapProbeAddr), Count: 1}, true
		}
		if s.probeStart > s.end {
			s.done = true
			scanDebugf(&s.cfg, "DEBUG [linear] done: probeStart=%d > end=%d\n", s.probeStart, s.end)
			return ScanTask{}, false
		}
		count := minU32(125, s.end-s.probeStart+1)
		scanDebugf(&s.cfg, "DEBUG [linear] phase=Probe probeStart=%d count=%d\n", s.probeStart, count)
		return ScanTask{Start: uint16(s.probeStart), Count: uint16(count)}, true
	case linearBackward:
		if s.backwardLow > s.backwardHigh {
			s.blockStart = s.originalBlockStart - s.backwardBest
			// blockEnd keeps the end of the successful probe set when the search started.
			s.phase = linearForward
			scanDebugf(&s.cfg, "DEBUG [linear] backward done: blockStart=%d blockEnd=%d -> phase Forward\n", s.blockStart, s.blockEnd)
			return s.Next()
		}
		mid := (s.backwardLow + s.backwardHigh) / 2
		chunkStart := s.originalBlockStart - mid
		if chunkStart < s.start {
			s.backwardHigh = mid - 1
			return s.Next()
		}
		scanDebugf(&s.cfg, "DEBUG [linear] phase=Backward mid=%d [%d,%d] (low=%d high=%d best=%d)\n", mid, chunkStart, chunkStart+mid-1, s.backwardLow, s.backwardHigh, s.backwardBest)
		return ScanTask{Start: uint16(chunkStart), Count: uint16(mid)}, true
	case linearForward:
		if s.blockEnd > s.end {
			s.probeStart = s.blockEnd
			s.phase = linearProbe
			s.hadProbeFailure = false
			scanDebugf(&s.cfg, "DEBUG [linear] block past end: probeStart=%d -> phase Probe\n", s.probeStart)
			return s.Next()
		}
		count := minU32(125, s.end-s.blockEnd+1)
		scanDebugf(&s.cfg, "DEBUG [linear] phase=Forward blockEnd=%d count=%d\n", s.blockEnd, count)
		return ScanTask{Start: uint16(s.blockEnd), Count: uint16(count)}, true
	case linearTail:
		if s.tailLow > s.tailHigh {
			if s.tailBest > 0 {
				s.probeStart = s.blockEnd + s.tailBest
			} else {
				s.probeStart = s.blockEnd + 125
			}
			if s.probeStart > s.blockEnd+1 {
				s.gapProbeAddr = s.blockEnd
				s.hasGapProbe = true
				scanDebugf(&s.cfg, "DEBUG [linear] tail done: gap probe pending at %d, probeStart=%d\n", s.gapProbeAddr, s.probeStart)
			} else {
				scanDebugf(&s.cfg, "DEBUG [linear] tail done: probeStart=%d -> phase Probe\n", s.probeStart)
			}
			s.phase = linearProbe
			s.hadProbeFailure = false
			return s.Next()
		}
		mid := (s.tailLow + s.tailHigh) / 2
		if mid < 1 {
			s.phase = linearProbe
			s.hadProbeFailure = false
			s.probeStart = s.blockEnd + 125
			if s.probeStart > s.blockEnd+1 {
				s.gapProbeAddr = s.blockEnd
				s.hasGapProbe = true
			}
			return s.Next()
		}
		remain := s.end - s.blockEnd + 1
		if mid > remain {
			s.tailHigh = mid - 1
			return s.Next()
		}
		scanDebugf(&s.cfg, "DEBUG [linear] phase=Tail blockEnd=%d mid=%d (low=%d high=%d best=%d)\n", s.blockEnd, mid, s.tailLow, s.tailHigh, s.tailBest)
		return ScanTask{Start: uint16(s.blockEnd), Count: uint16(mid)}, true
	}
	return ScanTask{}, false
}

func (s *linearStrategy) OnResult(task ScanTask, result ScanResult) {
	switch s.phase {
	case linearProbe:
		if s.lastTaskWasGapProbe {
			scanDebugf(&s.cfg, "DEBUG [linear] gap probe result: success=%v (no state change)\n", result.Success)
			s.lastTaskWasGapProbe = false
			return
		}
		if result.Success {
			newEnd := s.probeStart + uint32(task.Count)
			if s.hadProbeFailure {
				s.phase = linearBackward
				s.originalBlockStart = s.probeStart
				s.blockEnd = newEnd
				maxBack := s.originalBlockStart - s.start
				if maxBack > 125 {
					maxBack = 125
				}
				s.backwardLow = 1
				s.backwardHigh = maxBack
				s.backwardBest = 0
				scanDebugf(&s.cfg, "DEBUG [linear] probe success after fail -> Backward origStart=%d blockEnd=%d [1,%d]\n", s.originalBlockStart, s.blockEnd, maxBack)
			} else {
				s.phase = linearForward
				s.blockStart = s.probeStart
				s.blockEnd = newEnd
				scanDebugf(&s.cfg, "DEBUG [linear] probe success -> Forward block [%d,%d)\n", s.blockStart, s.blockEnd)
			}
		} else {
			s.hadProbeFailure = true
			prev := s.probeStart
			s.probeStart += 125
			scanDebugf(&s.cfg, "DEBUG [linear] probe fail: probeStart %d -> %d\n", prev, s.probeStart)
		}
	case linearBackward:
		mid := uint32(task.Count)
		if result.Success {
			s.backwardBest = mid
			s.backwardLow = mid + 1
			scanDebugf(&s.cfg, "DEBUG [linear] backward success: best=%d low=%d\n", s.backwardBest, s.backwardLow)
		} else {
			if mid > 1 {
				s.backwardHigh = mid - 1
			} else {
				s.backwardHigh = 0
			}
			scanDebugf(&s.cfg, "DEBUG [linear] backward fail: high=%d\n", s.backwardHigh)
		}
	case linearForward:
		if result.Success {
			s.blockEnd += uint32(task.Count)
			scanDebugf(&s.cfg, "DEBUG [linear] forward success: blockEnd=%d\n", s.blockEnd)
		} else {
			s.phase = linearTail
			s.tailLow = 1
			s.tailHigh = minU32(125, s.end-s.blockEnd+1)
			s.tailBest = 0
			scanDebugf(&s.cfg, "DEBUG [linear] forward fail -> Tail blockEnd=%d [1,%d]\n", s.blockEnd, s.tailHigh)
		}
	case linearTail:
		mid := uint32(task.Count)
		if result.Success {
			s.tailBest = mid
			s.tailLow = mid + 1
			scanDebugf(&s.cfg, "DEBUG [linear] tail success: best=%d low=%d\n", s.tailBest, s.tailLow)
		} else {
			if mid > 1 {
				s.tailHigh = mid - 1
			} else {
				s.tailHigh = 0
			}
			scanDebugf(&s.cfg, "DEBUG [linear] tail fail: high=%d\n", s.tailHigh)
		}
	}
}

func (s *linearStrategy) Done() bool {
	return s.done
}

// --- Boundary strategy (expand from seed, then binary-search boundaries) ---

var boundaryExpandSizes = []uint16{1, 2, 4, 8, 16, 32, 64, 125}

type boundaryPhase int

const (
	boundarySeed boundaryPhase = iota
	boundaryLeftExpand
	boundaryLeftBinary
	boundaryRightExpand
	boundaryRightBinary
	boundaryDone
)

// boundaryStrategy grows a known-good seed read into the full contiguous block around it.
// On each side it first expands with growing reads, then binary-searches the edge inside a
// window of at most MaxBlockSize registers; a window that turns out to be readable all the
// way is followed by the next one, so blocks of any size are covered.
//
// Positions are 32 bits wide so that blockEnd, an exclusive bound, can be 65536.
type boundaryStrategy struct {
	cfg        scanSettings
	start      uint32 // first address of the configured range
	end        uint32 // last address of the configured range (inclusive)
	blockStart uint32 // first address known to be readable
	blockEnd   uint32 // one past the last address known to be readable
	phase      boundaryPhase

	seedEmitted    bool
	leftExpandIdx  int
	rightExpandIdx int

	// Left binary search: the left edge lies in [leftLow, leftHigh]; leftFloor is the
	// lowest candidate of the current window.
	leftLow, leftHigh, leftFloor uint32
	// Right binary search: the last readable address lies in [rightLow-1, rightHigh];
	// rightCeil is the highest candidate of the current window.
	rightLow, rightHigh, rightCeil uint32
}

func newBoundaryStrategy(cfg scanSettings) *boundaryStrategy {
	return &boundaryStrategy{cfg: cfg}
}

func (s *boundaryStrategy) Name() string { return "boundary" }

func (s *boundaryStrategy) Init(cfg scanSettings) {
	s.cfg = cfg
	s.start = uint32(cfg.StartAddress)
	s.end = uint32(cfg.EndAddress)
	s.blockStart = uint32(cfg.SeedStart)
	s.blockEnd = uint32(cfg.SeedStart) + uint32(cfg.SeedCount)
	s.phase = boundarySeed
	s.seedEmitted = false
	s.leftExpandIdx = 0
	s.rightExpandIdx = 0
	// The seed must be 1..125 registers and lie fully inside [StartAddress, EndAddress].
	if cfg.SeedCount == 0 || cfg.SeedCount > MaxBlockSize || s.blockStart < s.start || s.blockEnd-1 > s.end {
		s.phase = boundaryDone
		scanDebugf(&s.cfg, "DEBUG [boundary] init: seed [%d,%d) not fully inside range [%d,%d], done\n", s.blockStart, s.blockEnd, s.start, s.end)
	} else {
		scanDebugf(&s.cfg, "DEBUG [boundary] init: seed [%d,%d) range [%d,%d]\n", s.blockStart, s.blockEnd, s.start, s.end)
	}
}

// boundaryTask builds the read of [from, to).
func boundaryTask(from, to uint32) ScanTask {
	return ScanTask{Start: uint16(from), Count: uint16(to - from)}
}

// openLeftWindow starts a binary search for the left edge in the window below blockStart.
func (s *boundaryStrategy) openLeftWindow() {
	s.phase = boundaryLeftBinary
	s.leftFloor = s.start
	if s.blockStart-s.start > MaxBlockSize {
		s.leftFloor = s.blockStart - MaxBlockSize
	}
	s.leftLow = s.leftFloor
	s.leftHigh = s.blockStart
	scanDebugf(&s.cfg, "DEBUG [boundary] -> phase LeftBinary [leftLow=%d leftHigh=%d]\n", s.leftLow, s.leftHigh)
}

// openRightWindow starts a binary search for the right edge in the window above blockEnd.
// It reports false when there is nothing left to search on the right.
func (s *boundaryStrategy) openRightWindow() bool {
	if s.blockEnd > s.end {
		return false
	}
	s.phase = boundaryRightBinary
	s.rightCeil = s.blockEnd + MaxBlockSize - 1
	if s.rightCeil > s.end {
		s.rightCeil = s.end
	}
	s.rightLow = s.blockEnd
	s.rightHigh = s.rightCeil
	scanDebugf(&s.cfg, "DEBUG [boundary] -> phase RightBinary [rightLow=%d rightHigh=%d]\n", s.rightLow, s.rightHigh)
	return true
}

func (s *boundaryStrategy) Next() (ScanTask, bool) {
	for {
		switch s.phase {
		case boundarySeed:
			if !s.seedEmitted {
				s.seedEmitted = true
				scanDebugf(&s.cfg, "DEBUG [boundary] phase=Seed emit [%d,%d)\n", s.blockStart, s.blockEnd)
				return boundaryTask(s.blockStart, s.blockEnd), true
			}
			s.phase = boundaryLeftExpand
			scanDebugf(&s.cfg, "DEBUG [boundary] -> phase LeftExpand blockStart=%d\n", s.blockStart)
		case boundaryLeftExpand:
			if s.leftExpandIdx >= len(boundaryExpandSizes) || s.blockStart <= s.start {
				s.openLeftWindow()
				continue
			}
			size := uint32(boundaryExpandSizes[s.leftExpandIdx])
			s.leftExpandIdx++
			// Clamp to the configured range: a read never starts below start.
			from := s.start
			if s.blockStart-s.start > size {
				from = s.blockStart - size
			}
			scanDebugf(&s.cfg, "DEBUG [boundary] phase=LeftExpand: [%d,%d)\n", from, s.blockStart)
			return boundaryTask(from, s.blockStart), true
		case boundaryLeftBinary:
			if s.leftLow >= s.leftHigh {
				// The whole window was readable and the range continues: search the next one.
				if s.blockStart == s.leftFloor && s.blockStart > s.start {
					s.openLeftWindow()
					continue
				}
				s.phase = boundaryRightExpand
				scanDebugf(&s.cfg, "DEBUG [boundary] -> phase RightExpand blockEnd=%d\n", s.blockEnd)
				continue
			}
			mid := (s.leftLow + s.leftHigh) / 2
			scanDebugf(&s.cfg, "DEBUG [boundary] phase=LeftBinary mid=%d [%d,%d)\n", mid, mid, s.blockStart)
			return boundaryTask(mid, s.blockStart), true
		case boundaryRightExpand:
			if s.rightExpandIdx >= len(boundaryExpandSizes) || s.blockEnd > s.end {
				if !s.openRightWindow() {
					s.phase = boundaryDone
				}
				continue
			}
			size := uint32(boundaryExpandSizes[s.rightExpandIdx])
			s.rightExpandIdx++
			// Clamp to the configured range: a read never ends past end.
			to := s.end + 1
			if to-s.blockEnd > size {
				to = s.blockEnd + size
			}
			scanDebugf(&s.cfg, "DEBUG [boundary] phase=RightExpand: [%d,%d)\n", s.blockEnd, to)
			return boundaryTask(s.blockEnd, to), true
		case boundaryRightBinary:
			if s.rightLow > s.rightHigh {
				// The whole window was readable and the range continues: search the next one.
				if s.blockEnd == s.rightCeil+1 && s.openRightWindow() {
					continue
				}
				s.phase = boundaryDone
				scanDebugf(&s.cfg, "DEBUG [boundary] -> phase Done block [%d,%d)\n", s.blockStart, s.blockEnd)
				continue
			}
			mid := (s.rightLow + s.rightHigh) / 2
			scanDebugf(&s.cfg, "DEBUG [boundary] phase=RightBinary mid=%d [%d,%d]\n", mid, s.blockEnd, mid)
			return boundaryTask(s.blockEnd, mid+1), true
		default:
			return ScanTask{}, false
		}
	}
}

func (s *boundaryStrategy) OnResult(task ScanTask, result ScanResult) {
	from := uint32(task.Start)
	to := from + uint32(task.Count)
	switch s.phase {
	case boundarySeed:
		if !result.Success {
			s.phase = boundaryDone
			scanDebugf(&s.cfg, "DEBUG [boundary] seed failed -> Done\n")
		} else {
			scanDebugf(&s.cfg, "DEBUG [boundary] seed success, block [%d,%d)\n", s.blockStart, s.blockEnd)
		}
	case boundaryLeftExpand:
		if result.Success {
			s.blockStart = from
			scanDebugf(&s.cfg, "DEBUG [boundary] left expand success: blockStart=%d\n", s.blockStart)
		} else {
			// The edge lies inside the failed read: search (from, blockStart].
			s.phase = boundaryLeftBinary
			s.leftFloor = from
			s.leftLow = from + 1
			s.leftHigh = s.blockStart
			scanDebugf(&s.cfg, "DEBUG [boundary] left expand fail -> LeftBinary leftLow=%d leftHigh=%d\n", s.leftLow, s.leftHigh)
		}
	case boundaryLeftBinary:
		if result.Success {
			s.blockStart = from
			s.leftHigh = from
			scanDebugf(&s.cfg, "DEBUG [boundary] left binary success: blockStart=%d\n", s.blockStart)
		} else {
			// [from, blockStart) holds an unreadable register: the edge is above from.
			s.leftLow = from + 1
			scanDebugf(&s.cfg, "DEBUG [boundary] left binary fail: leftLow=%d\n", s.leftLow)
		}
	case boundaryRightExpand:
		if result.Success {
			s.blockEnd = to
			scanDebugf(&s.cfg, "DEBUG [boundary] right expand success: blockEnd=%d\n", s.blockEnd)
		} else {
			// The edge lies inside the failed read [blockEnd, to): its last address is unconfirmed.
			s.phase = boundaryRightBinary
			s.rightCeil = to - 1
			s.rightLow = s.blockEnd
			s.rightHigh = to - 2
			scanDebugf(&s.cfg, "DEBUG [boundary] right expand fail -> RightBinary rightLow=%d rightHigh=%d\n", s.rightLow, s.rightHigh)
		}
	case boundaryRightBinary:
		if result.Success {
			s.blockEnd = to
			s.rightLow = to
			scanDebugf(&s.cfg, "DEBUG [boundary] right binary success: blockEnd=%d\n", s.blockEnd)
		} else {
			// [blockEnd, to) holds an unreadable register: the last readable address is below to-1.
			s.rightHigh = to - 2
			scanDebugf(&s.cfg, "DEBUG [boundary] right binary fail: rightHigh=%d\n", s.rightHigh)
		}
	case boundaryDone:
		// Terminal phase; executor should not deliver further results.
	}
}

func (s *boundaryStrategy) Done() bool {
	return s.phase == boundaryDone
}

func printScanWorstCaseHint(cfg config.ScanConfig, algo string, w io.Writer) {
	start := uint32(cfg.StartAddress)
	end := uint32(cfg.EndAddress)
	var rangeLen uint32
	if end >= start {
		rangeLen = end - start + 1
	}

	switch algo {
	case "safe":
		worst := rangeLen * 8 // at most 8 candidate sizes (125..1) per address
		_, _ = fmt.Fprintf(w, "Safe algo: worst case with no hits = %d addresses × 8 sizes = %d reads\n", rangeLen, worst)
	case "smart":
		initialChunks := (rangeLen + 124) / 125
		if initialChunks == 0 {
			initialChunks = 1
		}
		worst := 2*rangeLen - initialChunks
		_, _ = fmt.Fprintf(w, "Smart algo: worst case with no hits ≈ 2×%d − %d = %d reads\n", rangeLen, initialChunks, worst)
	case "deep":
		initialChunks := (rangeLen + 124) / 125
		if initialChunks == 0 {
			initialChunks = 1
		}
		phase1 := 2*rangeLen - initialChunks
		phase2 := uint32(500)
		_, _ = fmt.Fprintf(w, "Deep algo: worst case = phase 1 (smart) %d reads + phase 2 up to %d refinement = up to %d reads\n", phase1, phase2, phase1+phase2)
	case "stepped":
		step := uint32(cfg.Step)
		if step < 1 {
			step = 1000
		}
		var nSteps uint32
		if rangeLen > 0 {
			nSteps = (rangeLen-1)/step + 1
		}
		worst := nSteps * 6
		_, _ = fmt.Fprintf(w, "Stepped algo (step=%d): worst case with no hits = %d steps × 6 probes = %d reads\n", cfg.Step, nSteps, worst)
	case "linear":
		nBlocks := (rangeLen + 124) / 125
		if rangeLen > 0 && nBlocks == 0 {
			nBlocks = 1
		}
		_, _ = fmt.Fprintf(w, "Linear algo: worst case with no hits = %d probes (one per 125-block)\n", nBlocks)
	case "boundary":
		// 1 seed + left expand (up to 8) + left binary (log2 range) + right expand (8) + right binary (log2 range)
		_, _ = fmt.Fprintf(w, "Boundary algo: 1 seed + left/right expansion + binary search (depends on range)\n")
	case "sunspec":
		bases := len(sunspec.DefaultBaseAddresses)
		if cfg.SunSpecBases != "" {
			parsed, err := config.ParseSunSpecBases(cfg.SunSpecBases)
			if err == nil && len(parsed) > 0 {
				bases = len(parsed)
			}
		}
		if cfg.SunSpecBase > 0 {
			bases = 0
		}
		maxM := cfg.SunSpecMaxModels
		if maxM <= 0 {
			maxM = 256
		}
		_, _ = fmt.Fprintf(w, "SunSpec algo: worst case = %d base probes + up to %d models (header + body reads)\n", bases, maxM)
	}
}

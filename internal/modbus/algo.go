package modbus

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"

	"github.com/otfabric/modbusctl/internal/config"
)

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
	Init(cfg config.ScanConfig)
	Next() (ScanTask, bool)
	OnResult(task ScanTask, result ScanResult)
	Done() bool
}

// newScanStrategy returns the strategy for the given config's algo (normalized to lowercase).
func newScanStrategy(cfg config.ScanConfig) (ScanStrategy, error) {
	algo := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if algo == "" {
		algo = "safe"
	}
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
	default:
		return nil, fmt.Errorf("unsupported algo %q", cfg.Algo)
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
	cfg                      config.ScanConfig
	current                  uint16
	end                      uint16
	done                     bool
	candidates               []uint16
	candidateIndex           int
	hadFailureThisAddr       bool   // true if we had at least one failure at current address (Milestone D)
	leftBoundaryProbeAddr    uint16 // address to probe (0 is valid); use with leftBoundaryProbePending
	leftBoundaryProbePending bool
}

func newSafeStrategy(cfg config.ScanConfig) *safeStrategy {
	return &safeStrategy{cfg: cfg}
}

func (s *safeStrategy) Name() string { return "safe" }

func (s *safeStrategy) Init(cfg config.ScanConfig) {
	s.cfg = cfg
	s.current = cfg.StartAddress
	s.end = cfg.EndAddress
	s.done = false
	s.candidates = nil
	s.candidateIndex = 0
	s.hadFailureThisAddr = false
	s.leftBoundaryProbePending = false
	if s.cfg.Debug {
		fmt.Printf("DEBUG [safe] init: range [%d,%d]\n", s.current, s.end)
	}
}

func (s *safeStrategy) Next() (ScanTask, bool) {
	// Milestone D: one left-boundary probe when we succeeded after failure (ladder boundary)
	if s.leftBoundaryProbePending {
		s.leftBoundaryProbePending = false
		if s.cfg.Debug {
			fmt.Printf("DEBUG [safe] left-boundary probe: addr=%d count=1\n", s.leftBoundaryProbeAddr)
		}
		return ScanTask{Start: s.leftBoundaryProbeAddr, Count: 1}, true
	}
	if s.done || s.current > s.end {
		if s.cfg.Debug && s.current > s.end {
			fmt.Printf("DEBUG [safe] done: current=%d > end=%d\n", s.current, s.end)
		}
		return ScanTask{}, false
	}
	if s.candidates == nil || s.candidateIndex >= len(s.candidates) {
		s.candidates = candidateCountsSafe(s.current, s.end)
		s.candidateIndex = 0
		if len(s.candidates) == 0 {
			s.done = true
			if s.cfg.Debug {
				fmt.Printf("DEBUG [safe] no candidates at current=%d, done\n", s.current)
			}
			return ScanTask{}, false
		}
		if s.cfg.Debug {
			fmt.Printf("DEBUG [safe] candidates at current=%d: %v\n", s.current, s.candidates)
		}
	}
	count := s.candidates[s.candidateIndex]
	s.candidateIndex++
	return ScanTask{Start: s.current, Count: count}, true
}

func (s *safeStrategy) OnResult(task ScanTask, result ScanResult) {
	if result.Success {
		if s.hadFailureThisAddr && task.Start > 0 {
			s.leftBoundaryProbeAddr = task.Start - 1
			s.leftBoundaryProbePending = true
			if s.cfg.Debug {
				fmt.Printf("DEBUG [safe] scheduled left-boundary probe at addr=%d (after success at %d)\n", s.leftBoundaryProbeAddr, task.Start)
			}
		}
		prev := s.current
		s.current += task.Count
		s.candidates = nil
		s.candidateIndex = 0
		s.hadFailureThisAddr = false
		if s.cfg.Debug {
			fmt.Printf("DEBUG [safe] success: advance current %d -> %d (count=%d)\n", prev, s.current, task.Count)
		}
	} else {
		s.hadFailureThisAddr = true
		// Exhausted all candidates for this address; move to next
		if s.candidateIndex >= len(s.candidates) {
			prev := s.current
			s.current++
			s.candidates = nil
			s.candidateIndex = 0
			if s.cfg.Debug {
				fmt.Printf("DEBUG [safe] full failure at addr %d, advance current %d -> %d\n", prev, prev, s.current)
			}
		}
	}
}

func (s *safeStrategy) Done() bool {
	// Not done while a left-boundary probe is pending (must be returned before completion).
	if s.leftBoundaryProbePending {
		return false
	}
	return s.current > s.end
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
	cfg     config.ScanConfig
	queue   smartQueue
	visited map[uint32]bool // key: start<<16|count
}

func newSmartStrategy(cfg config.ScanConfig) *smartStrategy {
	return &smartStrategy{cfg: cfg, visited: make(map[uint32]bool)}
}

func (s *smartStrategy) Name() string { return "smart" }

func (s *smartStrategy) Init(cfg config.ScanConfig) {
	s.cfg = cfg
	s.visited = make(map[uint32]bool)
	s.queue = nil
	start := cfg.StartAddress
	end := cfg.EndAddress
	for start <= end {
		count := end - start + 1
		if count > MaxBlockSize {
			count = MaxBlockSize
		}
		heap.Push(&s.queue, Interval{Start: start, Count: count})
		start += count
	}
	if s.cfg.Debug {
		fmt.Printf("DEBUG [smart] init: range [%d,%d] initial intervals=%d\n", cfg.StartAddress, cfg.EndAddress, s.queue.Len())
	}
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
		if s.cfg.Debug {
			fmt.Printf("DEBUG [smart] pop: start=%d count=%d (queue remaining=%d)\n", iv.Start, iv.Count, s.queue.Len())
		}
		return ScanTask(iv), true
	}
	if s.cfg.Debug {
		fmt.Printf("DEBUG [smart] queue empty, done\n")
	}
	return ScanTask{}, false
}

func (s *smartStrategy) OnResult(task ScanTask, result ScanResult) {
	if result.Success {
		if s.cfg.Debug {
			fmt.Printf("DEBUG [smart] success: [%d,%d] recorded\n", task.Start, task.Start+task.Count-1)
		}
		return // interval recorded as readable (MCAP written by executor)
	}
	if task.Count > 1 {
		leftCount := task.Count / 2
		rightStart := task.Start + leftCount
		rightCount := task.Count - leftCount
		heap.Push(&s.queue, Interval{Start: task.Start, Count: leftCount})
		heap.Push(&s.queue, Interval{Start: rightStart, Count: rightCount})
		if s.cfg.Debug {
			fmt.Printf("DEBUG [smart] split fail: [%d,%d] -> left [%d,%d] right [%d,%d] (queue size=%d)\n",
				task.Start, task.Start+task.Count-1, task.Start, task.Start+leftCount-1, rightStart, rightStart+rightCount-1, s.queue.Len())
		}
	} else if s.cfg.Debug {
		fmt.Printf("DEBUG [smart] fail singleton: addr=%d unreadable\n", task.Start)
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
	cfg               config.ScanConfig
	phase             int // 1 = smart, 2 = refinement
	smart             *smartStrategy
	readableIntervals []Interval
	failedIntervals   []Interval // phase 1 failures for boundary evidence
	refinementQueue   []ScanTask
	refinementSeen    map[uint32]bool // dedup (start<<16|count)
}

func newDeepStrategy(cfg config.ScanConfig) *deepStrategy {
	return &deepStrategy{cfg: cfg, refinementSeen: make(map[uint32]bool)}
}

func (s *deepStrategy) Name() string { return "deep" }

func (s *deepStrategy) Init(cfg config.ScanConfig) {
	s.cfg = cfg
	s.phase = 1
	s.smart = newSmartStrategy(cfg)
	s.smart.Init(cfg)
	s.readableIntervals = nil
	s.failedIntervals = nil
	s.refinementQueue = nil
	s.refinementSeen = make(map[uint32]bool)
	if s.cfg.Debug {
		fmt.Printf("DEBUG [deep] init: phase 1 (smart), range [%d,%d]\n", cfg.StartAddress, cfg.EndAddress)
	}
}

func (s *deepStrategy) Next() (ScanTask, bool) {
	if s.phase == 1 {
		task, ok := s.smart.Next()
		if ok {
			return task, true
		}
		// Smart phase done; build refinement queue from readable intervals
		if s.cfg.Debug {
			fmt.Printf("DEBUG [deep] phase 1 done: readable=%d failed=%d, building refinement\n", len(s.readableIntervals), len(s.failedIntervals))
		}
		s.phase = 2
		s.buildRefinementQueue()
		if s.cfg.Debug {
			fmt.Printf("DEBUG [deep] phase 2: refinement queue size=%d\n", len(s.refinementQueue))
		}
	}
	// Phase 2: serve from refinement queue
	if len(s.refinementQueue) > 0 {
		task := s.refinementQueue[0]
		s.refinementQueue = s.refinementQueue[1:]
		if s.cfg.Debug {
			fmt.Printf("DEBUG [deep] refinement task: start=%d count=%d (remaining=%d)\n", task.Start, task.Count, len(s.refinementQueue))
		}
		return task, true
	}
	if s.cfg.Debug {
		fmt.Printf("DEBUG [deep] phase 2 done\n")
	}
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
			if s.cfg.Debug {
				fmt.Printf("DEBUG [deep] phase 1: readable + [%d,%d] (total=%d)\n", task.Start, task.Start+task.Count-1, len(s.readableIntervals))
			}
		} else {
			s.failedIntervals = append(s.failedIntervals, Interval(task))
			if s.cfg.Debug {
				fmt.Printf("DEBUG [deep] phase 1: failed + [%d,%d] (total=%d)\n", task.Start, task.Start+task.Count-1, len(s.failedIntervals))
			}
		}
	}
	// Phase 2: no further splitting, executor already wrote MCAP on success
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
	cfg           config.ScanConfig
	stepPositions []uint16
	stepIndex     int
	probeTasks    []ScanTask
	probeIndex    int
	hasHit        bool // true when a probe succeeded at this step (address 0 is valid)
	hitAddr       uint16
	expandIndex   int // -1 = not expanding, 0..5 = current expansion size index
}

func newSteppedStrategy(cfg config.ScanConfig) *steppedStrategy {
	return &steppedStrategy{cfg: cfg, expandIndex: -1}
}

func (s *steppedStrategy) Name() string { return "stepped" }

func (s *steppedStrategy) Init(cfg config.ScanConfig) {
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
	if s.cfg.Debug {
		fmt.Printf("DEBUG [stepped] init: range [%d,%d] step=%d halfOffset=%v positions=%d\n", start, end, step, cfg.StepHalfOffset, len(s.stepPositions))
	}
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
				if s.cfg.Debug {
					fmt.Printf("DEBUG [stepped] expansion: stepIndex=%d hitAddr=%d size=%d (expandIndex=%d)\n", s.stepIndex, s.hitAddr, size, s.expandIndex)
				}
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
		if s.cfg.Debug {
			fmt.Printf("DEBUG [stepped] probe: stepIndex=%d pos=%d probe %d/%d (start=%d count=%d)\n",
				s.stepIndex, s.stepPositions[s.stepIndex], s.probeIndex, len(s.probeTasks), task.Start, task.Count)
		}
		return task, true
	}
	if s.hasHit {
		s.expandIndex = 0
		maxEnd := s.maxExpandEnd()
		// Find first expansion size that fits within [StartAddress, EndAddress]
		for s.expandIndex < len(steppedExpandSizes) {
			size := steppedExpandSizes[s.expandIndex]
			if uint32(s.hitAddr)+uint32(size)-1 <= uint32(maxEnd) {
				if s.cfg.Debug {
					fmt.Printf("DEBUG [stepped] hit at stepIndex=%d hitAddr=%d, start expansion size=%d\n", s.stepIndex, s.hitAddr, size)
				}
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
		if s.cfg.Debug {
			fmt.Printf("DEBUG [stepped] all steps done\n")
		}
		return ScanTask{}, false
	}
	s.hasHit = false
	s.hitAddr = 0
	s.buildProbeTasks()
	if s.cfg.Debug {
		fmt.Printf("DEBUG [stepped] advance to stepIndex=%d pos=%d (%d probes)\n", s.stepIndex, s.stepPositions[s.stepIndex], len(s.probeTasks))
	}
	return s.Next()
}

func (s *steppedStrategy) OnResult(task ScanTask, result ScanResult) {
	if s.expandIndex >= 0 && s.expandIndex < len(steppedExpandSizes) {
		if result.Success {
			if s.cfg.Debug {
				fmt.Printf("DEBUG [stepped] expansion success: [%d,%d], advance to next step\n", task.Start, task.Start+task.Count-1)
			}
			s.hasHit = false
			s.hitAddr = 0
			s.expandIndex = -1
			s.stepIndex++
			s.buildProbeTasks()
		} else {
			s.expandIndex++
			if s.cfg.Debug {
				fmt.Printf("DEBUG [stepped] expansion fail size=%d (expandIndex=%d)\n", task.Count, s.expandIndex)
			}
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
		if s.cfg.Debug {
			fmt.Printf("DEBUG [stepped] probe hit: addr=%d (stepIndex=%d)\n", task.Start, s.stepIndex)
		}
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

type linearStrategy struct {
	cfg                config.ScanConfig
	start              uint16
	end                uint16
	phase              linearPhase
	probeStart         uint16
	hadProbeFailure    bool
	originalBlockStart uint16
	blockStart         uint16
	blockEnd           uint16
	// Backward binary search: find max K such that read(originalBlockStart-K, K) succeeds.
	backwardLow  uint16
	backwardHigh uint16
	backwardBest uint16
	// Tail binary search: find max count C such that read(blockEnd, C) succeeds.
	tailLow             uint16
	tailHigh            uint16
	tailBest            uint16
	done                bool
	hasGapProbe         bool // true when a gap probe is pending (address 0 is valid)
	gapProbeAddr        uint16
	lastTaskWasGapProbe bool // last Next() was gap probe; OnResult must not update probe state
}

func newLinearStrategy(cfg config.ScanConfig) *linearStrategy {
	return &linearStrategy{cfg: cfg}
}

func (s *linearStrategy) Name() string { return "linear" }

func (s *linearStrategy) Init(cfg config.ScanConfig) {
	s.cfg = cfg
	s.start = cfg.StartAddress
	s.end = cfg.EndAddress
	s.phase = linearProbe
	s.probeStart = cfg.StartAddress
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
	if s.cfg.Debug {
		fmt.Printf("DEBUG [linear] init: range [%d,%d] phase=Probe probeStart=%d\n", s.start, s.end, s.probeStart)
	}
}

func minU16(a, b uint16) uint16 {
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
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] gap probe: addr=%d (between block and next 125)\n", s.gapProbeAddr)
			}
			return ScanTask{Start: s.gapProbeAddr, Count: 1}, true
		}
		if s.probeStart > s.end {
			s.done = true
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] done: probeStart=%d > end=%d\n", s.probeStart, s.end)
			}
			return ScanTask{}, false
		}
		count := minU16(125, s.end-s.probeStart+1)
		if s.cfg.Debug {
			fmt.Printf("DEBUG [linear] phase=Probe probeStart=%d count=%d\n", s.probeStart, count)
		}
		return ScanTask{Start: s.probeStart, Count: count}, true
	case linearBackward:
		if s.backwardLow > s.backwardHigh {
			s.blockStart = s.originalBlockStart - s.backwardBest
			newEnd := uint32(s.originalBlockStart) + 125
			if newEnd > 65535 {
				newEnd = 65535
			}
			s.blockEnd = uint16(newEnd)
			s.phase = linearForward
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] backward done: blockStart=%d blockEnd=%d -> phase Forward\n", s.blockStart, s.blockEnd)
			}
			return s.Next()
		}
		mid := (s.backwardLow + s.backwardHigh) / 2
		chunkStart := s.originalBlockStart - mid
		if chunkStart < s.start {
			s.backwardHigh = mid - 1
			return s.Next()
		}
		if s.cfg.Debug {
			fmt.Printf("DEBUG [linear] phase=Backward mid=%d [%d,%d] (low=%d high=%d best=%d)\n", mid, chunkStart, chunkStart+mid-1, s.backwardLow, s.backwardHigh, s.backwardBest)
		}
		return ScanTask{Start: chunkStart, Count: mid}, true
	case linearForward:
		if s.blockEnd > s.end {
			s.probeStart = s.blockEnd
			s.phase = linearProbe
			s.hadProbeFailure = false
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] block past end: probeStart=%d -> phase Probe\n", s.probeStart)
			}
			return s.Next()
		}
		count := minU16(125, s.end-s.blockEnd+1)
		if s.cfg.Debug {
			fmt.Printf("DEBUG [linear] phase=Forward blockEnd=%d count=%d\n", s.blockEnd, count)
		}
		return ScanTask{Start: s.blockEnd, Count: count}, true
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
				if s.cfg.Debug {
					fmt.Printf("DEBUG [linear] tail done: gap probe pending at %d, probeStart=%d\n", s.gapProbeAddr, s.probeStart)
				}
			} else if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] tail done: probeStart=%d -> phase Probe\n", s.probeStart)
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
		if s.cfg.Debug {
			fmt.Printf("DEBUG [linear] phase=Tail blockEnd=%d mid=%d (low=%d high=%d best=%d)\n", s.blockEnd, mid, s.tailLow, s.tailHigh, s.tailBest)
		}
		return ScanTask{Start: s.blockEnd, Count: mid}, true
	}
	return ScanTask{}, false
}

func (s *linearStrategy) OnResult(task ScanTask, result ScanResult) {
	switch s.phase {
	case linearProbe:
		if s.lastTaskWasGapProbe {
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] gap probe result: success=%v (no state change)\n", result.Success)
			}
			s.lastTaskWasGapProbe = false
			return
		}
		if result.Success {
			newEnd := uint32(s.probeStart) + uint32(task.Count)
			if newEnd > 65535 {
				// Past valid address space; mark done
				s.done = true
				return
			}
			if s.hadProbeFailure {
				s.phase = linearBackward
				s.originalBlockStart = s.probeStart
				s.blockEnd = uint16(newEnd)
				maxBack := s.originalBlockStart - s.start
				if maxBack > 125 {
					maxBack = 125
				}
				s.backwardLow = 1
				s.backwardHigh = maxBack
				s.backwardBest = 0
				if s.cfg.Debug {
					fmt.Printf("DEBUG [linear] probe success after fail -> Backward origStart=%d blockEnd=%d [1,%d]\n", s.originalBlockStart, s.blockEnd, maxBack)
				}
			} else {
				s.phase = linearForward
				s.blockStart = s.probeStart
				s.blockEnd = uint16(newEnd)
				if s.cfg.Debug {
					fmt.Printf("DEBUG [linear] probe success -> Forward block [%d,%d)\n", s.blockStart, s.blockEnd)
				}
			}
		} else {
			s.hadProbeFailure = true
			prev := s.probeStart
			s.probeStart += 125
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] probe fail: probeStart %d -> %d\n", prev, s.probeStart)
			}
		}
	case linearBackward:
		mid := task.Count
		if result.Success {
			s.backwardBest = mid
			s.backwardLow = mid + 1
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] backward success: best=%d low=%d\n", s.backwardBest, s.backwardLow)
			}
		} else {
			if mid > 1 {
				s.backwardHigh = mid - 1
			} else {
				s.backwardHigh = 0
			}
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] backward fail: high=%d\n", s.backwardHigh)
			}
		}
	case linearForward:
		if result.Success {
			newEnd := uint32(s.blockEnd) + uint32(task.Count)
			if newEnd > 65535 {
				s.done = true
				return
			}
			s.blockEnd = uint16(newEnd)
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] forward success: blockEnd=%d\n", s.blockEnd)
			}
		} else {
			s.phase = linearTail
			s.tailLow = 1
			s.tailHigh = minU16(125, s.end-s.blockEnd+1)
			s.tailBest = 0
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] forward fail -> Tail blockEnd=%d [1,%d]\n", s.blockEnd, s.tailHigh)
			}
		}
	case linearTail:
		mid := task.Count
		if result.Success {
			s.tailBest = mid
			s.tailLow = mid + 1
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] tail success: best=%d low=%d\n", s.tailBest, s.tailLow)
			}
		} else {
			if mid > 1 {
				s.tailHigh = mid - 1
			} else {
				s.tailHigh = 0
			}
			if s.cfg.Debug {
				fmt.Printf("DEBUG [linear] tail fail: high=%d\n", s.tailHigh)
			}
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

type boundaryStrategy struct {
	cfg            config.ScanConfig
	start          uint16
	end            uint16
	blockStart     uint16
	blockEnd       uint16
	phase          boundaryPhase
	seedEmitted    bool
	leftExpandIdx  int
	leftLow        uint16
	leftHigh       uint16
	rightExpandIdx int
	rightLow       uint16
	rightHigh      uint16
}

func newBoundaryStrategy(cfg config.ScanConfig) *boundaryStrategy {
	return &boundaryStrategy{cfg: cfg}
}

func (s *boundaryStrategy) Name() string { return "boundary" }

func (s *boundaryStrategy) Init(cfg config.ScanConfig) {
	s.cfg = cfg
	s.start = cfg.StartAddress
	s.end = cfg.EndAddress
	s.blockStart = cfg.SeedStart
	s.blockEnd = cfg.SeedStart + cfg.SeedCount
	s.phase = boundarySeed
	s.seedEmitted = false
	s.leftExpandIdx = 0
	s.rightExpandIdx = 0
	// Seed must be fully inside [StartAddress, EndAddress]: SeedStart >= start, SeedStart+SeedCount-1 <= end
	// Also reject SeedCount == 0 or SeedCount > 125
	if cfg.SeedCount == 0 || cfg.SeedCount > 125 || s.blockStart < s.start || (uint32(s.blockEnd)-1 > uint32(s.end)) {
		s.phase = boundaryDone
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] init: seed [%d,%d) not fully inside range [%d,%d], done\n", s.blockStart, s.blockEnd, s.start, s.end)
		}
	} else if s.cfg.Debug {
		fmt.Printf("DEBUG [boundary] init: seed [%d,%d) range [%d,%d]\n", s.blockStart, s.blockEnd, s.start, s.end)
	}
}

func (s *boundaryStrategy) Next() (ScanTask, bool) {
	switch s.phase {
	case boundarySeed:
		if !s.seedEmitted {
			s.seedEmitted = true
			count := s.blockEnd - s.blockStart
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] phase=Seed emit [%d,%d) count=%d\n", s.blockStart, s.blockEnd, count)
			}
			return ScanTask{Start: s.blockStart, Count: count}, true
		}
		s.phase = boundaryLeftExpand
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] -> phase LeftExpand blockStart=%d\n", s.blockStart)
		}
		return s.Next()
	case boundaryLeftExpand:
		for s.leftExpandIdx < len(boundaryExpandSizes) {
			size := boundaryExpandSizes[s.leftExpandIdx]
			if s.blockStart < size {
				s.leftExpandIdx++
				continue
			}
			chunkStart := s.blockStart - size
			// Clamp to configured range: read must not start below start
			if chunkStart < s.start {
				clampCount := s.blockStart - s.start
				if clampCount < 1 {
					s.leftExpandIdx++
					continue
				}
				s.leftExpandIdx++
				if s.cfg.Debug {
					fmt.Printf("DEBUG [boundary] phase=LeftExpand clamp: [%d,%d)\n", s.start, s.start+clampCount)
				}
				return ScanTask{Start: s.start, Count: clampCount}, true
			}
			s.leftExpandIdx++
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] phase=LeftExpand: [%d,%d) size=%d\n", chunkStart, chunkStart+size, size)
			}
			return ScanTask{Start: chunkStart, Count: size}, true
		}
		s.phase = boundaryLeftBinary
		s.leftLow = s.start
		s.leftHigh = s.blockStart
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] -> phase LeftBinary [leftLow=%d leftHigh=%d]\n", s.leftLow, s.leftHigh)
		}
		return s.Next()
	case boundaryLeftBinary:
		if s.leftLow >= s.leftHigh {
			s.phase = boundaryRightExpand
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] -> phase RightExpand blockEnd=%d\n", s.blockEnd)
			}
			return s.Next()
		}
		mid := (s.leftLow + s.leftHigh) / 2
		count := s.blockStart - mid
		if count < 1 {
			s.leftLow = mid + 1
			return s.Next()
		}
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] phase=LeftBinary mid=%d count=%d [%d,%d)\n", mid, count, mid, mid+count)
		}
		return ScanTask{Start: mid, Count: count}, true
	case boundaryRightExpand:
		for s.rightExpandIdx < len(boundaryExpandSizes) {
			size := boundaryExpandSizes[s.rightExpandIdx]
			if uint32(s.blockEnd)+uint32(size) > 65535 {
				s.rightExpandIdx++
				continue
			}
			// Clamp to configured range: read must not end past end; never emit Count == 0
			if s.blockEnd > s.end {
				s.rightExpandIdx++
				continue
			}
			if s.blockEnd+size-1 > s.end {
				clampCount := s.end - s.blockEnd + 1
				if clampCount < 1 {
					s.rightExpandIdx++
					continue
				}
				s.rightExpandIdx++
				if s.cfg.Debug {
					fmt.Printf("DEBUG [boundary] phase=RightExpand clamp: [%d,%d)\n", s.blockEnd, s.blockEnd+clampCount)
				}
				return ScanTask{Start: s.blockEnd, Count: clampCount}, true
			}
			s.rightExpandIdx++
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] phase=RightExpand: [%d,%d) size=%d\n", s.blockEnd, s.blockEnd+size, size)
			}
			return ScanTask{Start: s.blockEnd, Count: size}, true
		}
		s.phase = boundaryRightBinary
		s.rightLow = s.blockEnd
		lastSize := boundaryExpandSizes[len(boundaryExpandSizes)-1]
		maxRight := uint32(s.blockEnd) + uint32(lastSize) - 1
		if maxRight > 65535 {
			maxRight = 65535
		}
		if uint32(s.end) < maxRight {
			maxRight = uint32(s.end)
		}
		s.rightHigh = uint16(maxRight)
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] -> phase RightBinary [rightLow=%d rightHigh=%d]\n", s.rightLow, s.rightHigh)
		}
		return s.Next()
	case boundaryRightBinary:
		if s.rightLow > s.rightHigh {
			s.phase = boundaryDone
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] -> phase Done block [%d,%d)\n", s.blockStart, s.blockEnd)
			}
			return ScanTask{}, false
		}
		mid := (uint32(s.rightLow) + uint32(s.rightHigh)) / 2
		mid16 := uint16(mid)
		count := mid16 - s.blockEnd + 1
		if count < 1 {
			s.rightLow = mid16 + 1
			return s.Next()
		}
		if uint32(s.blockEnd)+uint32(count)-1 > 65535 {
			s.rightHigh = mid16 - 1
			return s.Next()
		}
		if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] phase=RightBinary mid=%d count=%d [%d,%d)\n", mid16, count, s.blockEnd, s.blockEnd+count)
		}
		return ScanTask{Start: s.blockEnd, Count: count}, true
	case boundaryDone:
		return ScanTask{}, false
	}
	return ScanTask{}, false
}

func (s *boundaryStrategy) OnResult(task ScanTask, result ScanResult) {
	switch s.phase {
	case boundarySeed:
		if !result.Success {
			s.phase = boundaryDone
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] seed failed -> Done\n")
			}
		} else if s.cfg.Debug {
			fmt.Printf("DEBUG [boundary] seed success, block [%d,%d)\n", s.blockStart, s.blockEnd)
		}
	case boundaryLeftExpand:
		if result.Success {
			s.blockStart = task.Start
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] left expand success: blockStart=%d\n", s.blockStart)
			}
		} else {
			s.phase = boundaryLeftBinary
			s.leftLow = task.Start
			s.leftHigh = s.blockStart
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] left expand fail -> LeftBinary leftLow=%d leftHigh=%d\n", s.leftLow, s.leftHigh)
			}
		}
	case boundaryLeftBinary:
		if result.Success {
			s.blockStart = task.Start
			s.leftHigh = task.Start
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] left binary success: blockStart=%d\n", s.blockStart)
			}
		} else {
			s.leftLow = task.Start + task.Count
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] left binary fail: leftLow=%d\n", s.leftLow)
			}
		}
	case boundaryRightExpand:
		if result.Success {
			s.blockEnd = task.Start + task.Count
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] right expand success: blockEnd=%d\n", s.blockEnd)
			}
		} else {
			// Per spec §7.5: failure in right expand → switch to right binary search
			s.phase = boundaryRightBinary
			s.rightLow = s.blockEnd
			lastSize := boundaryExpandSizes[len(boundaryExpandSizes)-1]
			maxRight := uint32(s.blockEnd) + uint32(lastSize) - 1
			if maxRight > 65535 {
				maxRight = 65535
			}
			if uint32(s.end) < maxRight {
				maxRight = uint32(s.end)
			}
			s.rightHigh = uint16(maxRight)
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] right expand fail -> RightBinary rightLow=%d rightHigh=%d\n", s.rightLow, s.rightHigh)
			}
		}
	case boundaryRightBinary:
		if result.Success {
			s.blockEnd = task.Start + task.Count
			s.rightLow = task.Start + task.Count
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] right binary success: blockEnd=%d\n", s.blockEnd)
			}
		} else {
			// tried read(blockEnd, count) up to inclusive task.Start+task.Count-1; max good is one less
			if task.Count >= 2 {
				s.rightHigh = task.Start + task.Count - 2
			} else if task.Start > 0 {
				s.rightHigh = task.Start - 1
			}
			if s.cfg.Debug {
				fmt.Printf("DEBUG [boundary] right binary fail: rightHigh=%d\n", s.rightHigh)
			}
		}
	}
}

func (s *boundaryStrategy) Done() bool {
	return s.phase == boundaryDone
}


func printScanWorstCaseHint(cfg config.ScanConfig, algo string) {
	start := uint32(cfg.StartAddress)
	end := uint32(cfg.EndAddress)
	var rangeLen uint32
	if end >= start {
		rangeLen = end - start + 1
	}

	switch algo {
	case "safe":
		worst := rangeLen * 8 // at most 8 candidate sizes (125..1) per address
		fmt.Printf("Safe algo: worst case with no hits = %d addresses × 8 sizes = %d reads\n", rangeLen, worst)
	case "smart":
		initialChunks := (rangeLen + 124) / 125
		if initialChunks == 0 {
			initialChunks = 1
		}
		worst := 2*rangeLen - initialChunks
		fmt.Printf("Smart algo: worst case with no hits ≈ 2×%d − %d = %d reads\n", rangeLen, initialChunks, worst)
	case "deep":
		initialChunks := (rangeLen + 124) / 125
		if initialChunks == 0 {
			initialChunks = 1
		}
		phase1 := 2*rangeLen - initialChunks
		phase2 := uint32(500)
		fmt.Printf("Deep algo: worst case = phase 1 (smart) %d reads + phase 2 up to %d refinement = up to %d reads\n", phase1, phase2, phase1+phase2)
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
		fmt.Printf("Stepped algo (step=%d): worst case with no hits = %d steps × 6 probes = %d reads\n", cfg.Step, nSteps, worst)
	case "linear":
		nBlocks := (rangeLen + 124) / 125
		if rangeLen > 0 && nBlocks == 0 {
			nBlocks = 1
		}
		fmt.Printf("Linear algo: worst case with no hits = %d probes (one per 125-block)\n", nBlocks)
	case "boundary":
		// 1 seed + left expand (up to 8) + left binary (log2 range) + right expand (8) + right binary (log2 range)
		fmt.Printf("Boundary algo: 1 seed + left/right expansion + binary search (depends on range)\n")
	}
}

# Scan algorithms (in-depth)

This document specifies the Modbus register scan algorithms supported by modbusctl. It is written so that each algorithm can be re-implemented outside this codebase without reading the source. All algorithms share the same **executor** and **strategy interface**; only the policy for choosing the next read and reacting to results differs.

---

## 1. Executor and strategy interface

### 1.1 Input configuration and range

Every algorithm receives a scan configuration with at least:

| Parameter    | Type   | Description |
|-------------|--------|-------------|
| `StartAddress` | uint16 | First register address to consider (inclusive). |
| `EndAddress`   | uint16 | Last register address to consider (inclusive). |
| `Function`     | uint8  | Modbus function code (e.g. 3 = holding, 4 = input). |
| `Delay`        | uint16 | Optional delay in ms between requests. |

Additional parameters (e.g. `Step` for stepped) are described per algorithm.

**Empty range:** If **StartAddress > EndAddress**, the strategy is immediately done and emits no tasks. Implementations must treat this as a valid configuration and return no task from the first Next() (or from Init ensure Done() is true).

### 1.2 Types

- **ScanTask**: `{ Start: uint16, Count: uint16 }` — a single read request. Every task must satisfy **1 ≤ Count ≤ 125** (Modbus limit). No task with Count 0 is valid.
- **ScanResult**: `{ Success, Start, Count, Data, RequestTimestamp, ResponseTimestamp, Err, OutcomeType, ExceptionCode, RTTNanos }` — outcome of executing a ScanTask. **Invariant:** **Success == true** if and only if **OutcomeType == success** (a normal Modbus read response). All other OutcomeTypes (exception, timeout, transport_error, protocol_error, unknown) imply Success == false. **OutcomeType** classifies the result; when `OutcomeType == exception`, **ExceptionCode** is the Modbus exception code (e.g. 0x02 Illegal Data Address). **RTTNanos** is response timestamp minus request timestamp. Strategies can branch on OutcomeType (e.g. do not retry on exception; retry on timeout).
- **Interval**: `{ Start: uint16, Count: uint16 }` — a contiguous range (used internally by some strategies).

### 1.3 Strategy interface

A scan strategy implements:

- **Init(cfg)** — Initialize state from the scan config (e.g. set current position, seed queues).
- **Next() → (ScanTask, bool)** — Return the next task to execute, or `(_, false)` when there is none.
- **OnResult(task, result)** — Update internal state given the result of executing `task`.
- **Done() → bool** — Return true when the strategy has no more work.

The **executor** loop is:

```
connect to device
open output file, write header
strategy.Init(cfg)
while !strategy.Done():
    task, ok := strategy.Next()
    if !ok: break
    result := executeRead(task)   // perform Modbus read, record timestamps
    strategy.OnResult(task, result)
    if result.Success:
        append result to output file
        optionally print block info
    sleep(cfg.Delay)
print summary stats
```

Only **successful** reads are written to the capture file. The strategy decides *what* to read next; the executor handles connection, timing, and I/O.

### 1.4 Modbus constraints

- Valid register addresses: **0–65535**.
- Maximum read size per request: **125** registers (Modbus limit). All algorithms cap any single read at 125.
- A read of `count` registers starting at `start` covers addresses `[start, start+count-1]` inclusive. A read may **fail** because the range is unsupported (e.g. Modbus exception) or because of timeout, transport, or protocol errors; strategies may interpret these differently using **OutcomeType**.

---

## 2. Algorithm: **safe**

**Goal:** Conservative, predictable scan. Linear pass over the range; at each position try the largest allowed block first, then smaller sizes until one succeeds or all fail. No refinement, no retry storms.

### 2.1 Parameters

- Uses only the common config: `StartAddress`, `EndAddress`, `Function`, `Delay`.

### 2.2 Constants

- **Candidate sizes** (in order, descending): `[125, 64, 32, 16, 8, 4, 2, 1]`.
- **MaxBlockSize** = 125.

### 2.3 State

- `current` — next address to try (initially `StartAddress`).
- `end` — `EndAddress`.
- `candidates` — list of sizes that fit in `[current, end]` from the candidate list (only sizes ≤ `end - current + 1`).
- `candidateIndex` — index into `candidates` for the next size to try (0 when starting at a new address).
- `done` — set when `current > end`.
- **`leftBoundaryProbePending`** (bool) — true when a one-off left-boundary probe has been scheduled and must be returned by Next() before completion.
- **`leftBoundaryProbeAddr`** (uint16) — when `leftBoundaryProbePending` is true, the address to probe (task is `(leftBoundaryProbeAddr, 1)`).
- **`hadFailureThisAddr`** (bool) — true if we have seen at least one failure at the current `current` address in this ladder.

### 2.4 Init

- `current = StartAddress`
- `end = EndAddress`
- `done = false`
- `candidates = nil`, `candidateIndex = 0`
- `leftBoundaryProbePending = false`, `hadFailureThisAddr = false`

### 2.5 Next()

1. **If `leftBoundaryProbePending`**: return task `{ Start: leftBoundaryProbeAddr, Count: 1 }` and set `leftBoundaryProbePending = false`. Do not advance `current`.
2. If `done` or `current > end`: return no task.
3. If `candidates` is nil or `candidateIndex >= len(candidates)`:
   - Build `candidates`: from `[125, 64, 32, 16, 8, 4, 2, 1]` take every size ≤ `end - current + 1`.
   - Set `candidateIndex = 0`.
   - If `candidates` is empty, set `done = true` and return no task.
4. `count = candidates[candidateIndex]`, increment `candidateIndex`.
5. Return task `{ Start: current, Count: count }`.

### 2.6 OnResult(task, result)

- If **success**:
  - If we had at least one failure at this address (`hadFailureThisAddr`) and `task.Start > 0`, schedule one **left-boundary probe**: set `leftBoundaryProbeAddr = task.Start − 1`, `leftBoundaryProbePending = true`. The probe is a **one-off refinement**: its result is recorded normally by the executor, but it **does not alter** `current` and **does not restart** candidate scanning at that address.
  - Then `current += task.Count`; set `candidates = nil`, `candidateIndex = 0`, `hadFailureThisAddr = false`.
- If **failure**: set `hadFailureThisAddr = true`; if `candidateIndex >= len(candidates)` (all sizes tried at this address), set `current += 1`, `candidates = nil`, `candidateIndex = 0`.

### 2.7 Done()

- Return true **only when** `current > end` **and** `!leftBoundaryProbePending`. (The pending boundary probe must be returned and processed before the strategy is done.)

### 2.8 Worst-case (no successful reads)

- At each address we try at most 8 sizes. So: **rangeLen × 8** reads, where `rangeLen = EndAddress - StartAddress + 1`.

### 2.9 Example (reduced range)

- Config: start=0, end=10.
- Try (0, 10) — fail (10 < 125, so count 10 is used from candidates). Then (0, 8), (0, 4), (0, 2), (0, 1). Suppose (0, 1) succeeds. Because we had a failure at this address and task.Start=0, no left-boundary probe is scheduled (task.Start > 0 required). Advance: current=1.
- If instead (1, 8) failed and (1, 4) succeeded: we had failure at this address and task.Start=1 > 0, so schedule left-boundary probe (0, 1). Then current += 4 → current=5. Next() returns (0, 1) first; after OnResult for (0, 1), current remains 5 (the boundary probe does not alter current). Then normal ladder continues from current=5.

---

## 3. Algorithm: **smart**

**Goal:** Discover readable regions efficiently with divide-and-conquer. Start with large (125-register) chunks; only split a chunk when a read fails, and only down to single-register granularity. Avoids sliding one address at a time when large regions fail. **Note:** The priority queue (smaller intervals first) makes smart more refinement-oriented than a pure coarse-first traversal; boundaries are discovered sooner.

### 3.1 Parameters

- Uses only the common config: `StartAddress`, `EndAddress`, `Function`, `Delay`.

### 3.2 Constants

- **MaxBlockSize** = 125.

### 3.3 State

- `queue` — **Priority queue** (min-heap) of **Interval** `{ Start, Count }`. **Ordering:** primary key = smaller `Count`; **secondary key** (tie-break) = smaller `Start`. This makes processing order deterministic: smaller intervals are preferred to discover boundaries sooner; ties are broken by Start.
- `visited` — set (or map) of intervals already issued, keyed e.g. by `(Start << 16) | Count` to avoid duplicates.

### 3.4 Init

- Clear `queue` and `visited`.
- Build initial intervals over `[StartAddress, EndAddress]` in 125-sized chunks (last chunk may be smaller):
  - `s = StartAddress`
  - While `s <= EndAddress`: `count = min(125, EndAddress - s + 1)`; push `{ Start: s, Count: count }` onto the heap; `s += count`.

### 3.5 Next()

1. While `queue` is not empty:
   - Pop the interval `iv` with smallest `Count` from the heap (ties broken by smaller `Start`).
   - Compute key = e.g. `(iv.Start << 16) | iv.Count`. If key is in `visited`, skip (pop next).
   - Mark key as visited.
   - Return task `{ Start: iv.Start, Count: iv.Count }`.
2. Return no task.

### 3.6 OnResult(task, result)

- If **success**: do nothing (executor records the block).
- If **failure**:
  - If `task.Count > 1`: split into two intervals:
    - `leftCount = task.Count / 2`
    - `rightStart = task.Start + leftCount`
    - `rightCount = task.Count - leftCount`
    - Push `{ Start: task.Start, Count: leftCount }` and `{ Start: rightStart, Count: rightCount }` onto the heap.
  - If `task.Count == 1`: do nothing (address marked unreadable).

### 3.7 Done()

- Return `len(queue) == 0`.

### 3.8 Worst-case (no successful reads)

- Initial chunks: `ceil(rangeLen / 125)`. Each failed chunk of size > 1 produces two new chunks. Upper bound used in implementation: **2×rangeLen − initialChunks** reads.

### 3.9 Example

- Config: start=0, end=249. Initial queue: two chunks — [0,125] and [125,124] (since 125+124=249). No third chunk at 250 because 250 > 249.
- After the initial failures, the heap contains four smaller intervals; **subsequent processing order is by heap order** (smallest Count first, then smallest Start on ties), not insertion order. So the next popped tasks might be [0,62], [125,62], [62,63], [187,62] depending on Count then Start. Continue until all chunks are size 1 and processed.

---

## 4. Algorithm: **deep**

**Goal:** Same as smart for phase 1; then a second phase that refines around the **boundaries** of every successfully read interval with small probes, to find exact edges and small islands. Capped to avoid excessive traffic.

### 4.1 Parameters

- Uses only the common config: `StartAddress`, `EndAddress`, `Function`, `Delay`.

### 4.2 Constants

- **MaxBlockSize** = 125.
- **deepRefinementWindow** = 8 (registers to each side of a boundary; evidence-driven only at boundaries).
- **deepRefinementCap** = 500 (max refinement tasks).
- **deepRefinementCounts** = [1, 2, 4, 8].

### 4.3 State

- `phase` — 1 (smart) or 2 (refinement).
- `smart` — full **smart** strategy instance (same state as in section 3).
- `readableIntervals` — list of intervals that were successfully read in phase 1.
- `failedIntervals` — list of intervals that failed in phase 1 (for boundary evidence).
- `refinementQueue` — list of ScanTasks for phase 2.
- `refinementSeen` — set of (Start, Count) already in queue, to deduplicate.

### 4.4 Init

- Set `phase = 1`.
- Create and init **smart** strategy with the same config.
- `readableIntervals = []`, `failedIntervals = []`, `refinementQueue = []`, `refinementSeen = {}`.

### 4.5 Next()

1. If `phase == 1`: (task, ok) := smart.Next(). If ok, return (task, true). If !ok, set phase = 2, call **buildRefinementQueue** (below), then fall through to step 2 (do not return no task until phase 2 has been checked).
2. If `phase == 2`: if `refinementQueue` is not empty, pop the first task and return it; else return no task.

### 4.6 buildRefinementQueue() (evidence-driven)

- Refine **only where there is evidence of a boundary**: a readable interval edge that has **failed-neighbor evidence**. Define evidence as: **a failed interval whose covered address range intersects the window [edge − 1, edge + 1]** (or the same idea: any failed interval that overlaps the one-register window around the edge). For each interval `iv` in `readableIntervals` (until refinement queue size ≥ deepRefinementCap):
  - `intervalStart = iv.Start`, `intervalEnd = iv.Start + iv.Count - 1`.
  - **Left boundary**: only if `intervalStart > StartAddress` and there exists a failed interval whose range intersects `[intervalStart − 1, intervalStart + 1]`. Then for start in `[intervalStart − 8, intervalStart + 8]` clamped to config, for each count in [1,2,4,8] such that the read stays in range, if (start, count) not in `refinementSeen`, add task and mark.
  - **Right boundary**: only if `intervalEnd < EndAddress` and there exists a failed interval whose range intersects `[intervalEnd − 1, intervalEnd + 1]`. Same ±8 window and counts.
- If no failed intervals were recorded, no refinement tasks are added (no evidence).

### 4.7 OnResult(task, result)

- If `phase == 1`: call `smart.OnResult(task, result)`. If `result.Success`, append `{ Start: task.Start, Count: task.Count }` to `readableIntervals`. If **failure**, append to `failedIntervals`.
- If `phase == 2`: do nothing (executor already records success).

### 4.8 Done()

- If `phase == 1`: return **false**. (Completion of phase 1 is detected inside Next() when smart.Next() returns no task; then phase is set to 2. Therefore Done() deliberately remains false until phase 2 has been evaluated.)
- If `phase == 2`: return `len(refinementQueue) == 0`.

### 4.9 Worst-case

- Phase 1: same as smart, e.g. **2×rangeLen − initialChunks**.
- Phase 2: up to **500** refinement reads.
- Total: **phase1 + 500**.

### 4.10 Example

- Phase 1 finds readable [100, 224] (125 regs) and [300, 324] (25 regs). Phase 2 adds probes only where a failed interval is adjacent (evidence-driven). For [100, 224]: left boundary window 92–108 (±8 around 100), right boundary window 216–232 (±8 around 224). For [300, 324]: left 292–308, right 316–332. Counts used: [1, 2, 4, 8] only.

---

## 5. Algorithm: **stepped**

**Goal:** Quick pass over a large range by probing only at **step** positions (e.g. 0, 1000, 2000, …). At each step, run a small set of probes; if any succeed, “expand” from that address with larger blocks to get maximum coverage at that hotspot, then move to the next step.

### 5.1 Parameters

- **Step** (uint16) — distance between step positions (default 1000). **Must be ≥ 1** when algo is stepped; if Step == 0, **treat strategy as done immediately** (emit no tasks). This keeps the algorithm spec self-contained.
- **StepHalfOffset** (bool) — when true, also add step positions at `start+step/2`, `start+step+step/2`, … (so probes run at 0, 50, 100, 150, … for step=100). Positions are deduplicated and sorted.
- Plus common config: `StartAddress`, `EndAddress`, `Function`, `Delay`.

### 5.2 Constants

- **Probe counts** at each step: 1 and 2 registers.
- **Probe positions** at step `pos`: `pos-1`, `pos`, `pos+1` (clamped so that the read stays within [0, 65535]; probe addresses may extend to StartAddress−1 and EndAddress+1 for boundary detection only).
- **steppedExpandSizes** = [125, 64, 32, 16, 8, 4] — sizes to try for expansion from a hit address. **Expansion reads are strictly clamped** to [StartAddress, EndAddress]: a read from hitAddr with size must satisfy hitAddr + size − 1 ≤ EndAddress (no expansion past the configured end).

### 5.3 State

- `stepPositions` — list of step positions (addresses) in order.
- `stepIndex` — current step (0-based).
- `probeTasks` — list of ScanTasks for the current step (up to 6: 3 positions × 2 counts).
- `probeIndex` — index into `probeTasks`.
- **`hasHit`** — true when any probe at the current step succeeded (use this, not `hitAddr == 0`, because **address 0 is valid** in Modbus; a hit at 0 would set hitAddr=0 and must still trigger expansion).
- `hitAddr` — address where a probe succeeded (meaningful when hasHit is true).
- `expandIndex` — 0..len(steppedExpandSizes)-1 when expanding from hitAddr, or -1 when not expanding.

### 5.4 Init

- If **Step == 0**, treat the strategy as done immediately: set `stepPositions = []` (or equivalent) and return; do not build positions. No tasks will be emitted.
- Build **primary** step positions: for `pos = StartAddress`; while `pos <= EndAddress`, append `pos`, then `pos += Step` (stop if overflow or past end).
- If **StepHalfOffset** is true: also build positions at `start + Step/2 + k×Step` for k = 0, 1, … (i.e. `StartAddress + Step/2`, `StartAddress + Step/2 + Step`, …). For odd Step, **Step/2** uses integer division (e.g. Step=1000 → half at 500).
- Discard any position outside [StartAddress, EndAddress] (or keep only those that fit the configured range for step alignment).
- **Deduplicate** the combined list and **sort ascending**.
- `stepIndex = 0`, `probeIndex = 0`, `hasHit = false`, `hitAddr = 0`, `expandIndex = -1`.
- Call **buildProbeTasks** for the current step.

### 5.5 buildProbeTasks()

- Clear `probeTasks` and set `probeIndex = 0`.
- If `stepIndex >= len(stepPositions)`, return.
- `pos = stepPositions[stepIndex]`.
- Define minAddr/maxAddr: e.g. if StartAddress > 0 then minAddr = StartAddress−1 else 0; if EndAddress < 65535 then maxAddr = EndAddress+1 else 65535 (clamp to 65535).
- For each of `pos-1`, `pos`, `pos+1` (clamped to [minAddr, maxAddr], and only if within 0..65535), for each count in {1, 2}: if read [addr, addr+count-1] would not exceed maxAddr, append `{ Start: addr, Count: count }` to `probeTasks`.

### 5.6 Next()

1. If `stepIndex >= len(stepPositions)`: return no task.
2. **Expansion**: if `expandIndex >= 0` and `expandIndex < len(steppedExpandSizes)`:
   - Let `maxEnd` = **EndAddress** (expansion is strictly within configured range; no read may end past EndAddress).
   - For each size in steppedExpandSizes from expandIndex: if `hitAddr + size - 1 <= maxEnd`, return task `{ Start: hitAddr, Count: size }` (OnResult will increment expandIndex).
   - If no size fits, clear hitAddr and expandIndex, advance stepIndex, rebuild probe tasks, recurse Next().
3. If there is a task in `probeTasks` at `probeIndex`: return it and increment `probeIndex`.
4. If **hasHit**: set `expandIndex = 0`, return task `{ Start: hitAddr, Count: steppedExpandSizes[0] }` (if it fits within maxEnd).
5. Advance `stepIndex`; if past end return no task. Set `hasHit = false`, `hitAddr = 0`, buildProbeTasks for new step, recurse Next().

### 5.7 OnResult(task, result)

- If we are in expansion (expandIndex >= 0):
  - If **success**: clear `hasHit` and hitAddr, set expandIndex = -1, advance stepIndex, buildProbeTasks, done with this step.
  - If **failure**: increment expandIndex; if expandIndex >= len(steppedExpandSizes), clear hasHit and hitAddr and expandIndex, advance stepIndex, buildProbeTasks.
- If we are in probe phase: if **success** and **not hasHit**, set `hasHit = true`, `hitAddr = task.Start`.

### 5.8 Done()

- True when stepIndex >= len(stepPositions) and not in the middle of expansion or probes or a pending hit (i.e. **hasHit** is false when all probes at current step are consumed).

### 5.9 Worst-case (no successful reads)

- **Exact:** **6 × len(stepPositions)** reads (each step position gets up to 6 probes; no expansion if no hit). This is correct for both StepHalfOffset = false and true, since len(stepPositions) already reflects half-offset and deduplication.
- When **StepHalfOffset = false**: len(stepPositions) = ceil((EndAddress − StartAddress + 1) / Step) for non-empty range (or 0 if range empty). So worst-case = 6 × ceil(rangeLen / Step).
- When **StepHalfOffset = true**: len(stepPositions) is larger (roughly up to twice the base count, minus deduplication); use the actual length for the formula.

### 5.10 Example

- start=0, end=1999, step=1000. Step positions: [0, 1000].
- At step 0: run the six probes (e.g. (0,1), (0,2), (1,1), (1,2), (2,1), (2,2)). **If all six probes fail**, then stepIndex=1 and we move to the next step. **If any probe succeeds**, set hasHit=true, hitAddr=that task’s Start; then expansion from that hit address (reads strictly within [0, 1999]). At step 1000: again 6 probes; all fail → done. Total 12 reads if no hits.

**Range note:** Only the **micro-probes** (pos±1, count 1 or 2) may use addresses slightly outside [StartAddress, EndAddress] for boundary detection. **Expansion** reads are strictly clamped to [StartAddress, EndAddress].

---

## 6. Algorithm: **linear**

**Goal:** Find maximum contiguous readable blocks aligned to 125-register boundaries. Probe 125-sized blocks; when one succeeds, extend forward with more 125s; when the next 125 fails, binary-search for the maximum tail (1..125). When a 125 succeeds *after* a previous 125 failed, binary-search backwards to find the real start of the readable region, then continue forward and tail as above.

### 6.1 Parameters

- Uses only the common config: `StartAddress`, `EndAddress`, `Function`, `Delay`.

### 6.2 Phases

- **Probe** — try 125-block at current position; on success go to Forward or Backward; on failure advance by 125.
- **Backward** — (only if previous 125 failed) binary-search for maximum backward extent from the successful 125 start; then go to Forward.
- **Forward** — repeatedly try next 125 at blockEnd; on success extend blockEnd; on failure go to Tail.
- **Tail** — binary-search for maximum count readable at blockEnd; then go back to Probe.

### 6.3 State

- `start`, `end` — from config.
- `phase` — Probe | Backward | Forward | Tail.
- `probeStart` — next 125-aligned position to try in Probe (e.g. start, start+125, …).
- `hadProbeFailure` — true if the previous probe (125) failed (used to decide Backward vs Forward).
- `originalBlockStart` — start address of the 125-block that succeeded after a failure (for Backward).
- `blockStart`, `blockEnd` — current contiguous readable region [blockStart, blockEnd) (blockEnd exclusive).
- Backward binary search: `backwardLow`, `backwardHigh`, `backwardBest` (search for max K such that read(originalBlockStart−K, K) succeeds).
- Tail binary search: `tailLow`, `tailHigh`, `tailBest` (search for max count C such that read(blockEnd, C) succeeds). After tail search, the **confirmed readable region** is [blockStart, blockEnd + tailBest) (blockEnd is not updated during tail; tailBest is the largest successful count at blockEnd).
- `done` — set when probeStart > end and we exit Probe.
- **`hasGapProbe`** (bool) — true when a gap probe is pending (do not use address as sentinel: **address 0 is valid**). **`gapProbeAddr`** (uint16) — when hasGapProbe is true, the address to probe (task is (gapProbeAddr, 1)).
- **`lastTaskWasGapProbe`** (bool) — true when the last task returned by Next() was the gap probe; OnResult must then **not** update probe/forward/backward/tail state (the gap probe is observation-only for the strategy; the executor still records success to MCAP).

### 6.4 Init

- `phase = Probe`, `probeStart = StartAddress`, `hadProbeFailure = false`, `done = false`, `hasGapProbe = false`, `lastTaskWasGapProbe = false`.
- Clear block/backward/tail fields (blockStart=0, blockEnd=0, backwardLow/High/Best=0, tailLow/High/Best=0).

### 6.5 Next() — Probe

- If **hasGapProbe** is true: set `lastTaskWasGapProbe = true`, clear hasGapProbe, return `(gapProbeAddr, 1)`. This detects small readable islands in the gap between the last block and the next 125-block.
- If `probeStart > end`: set `done = true`, return no task.
- `count = min(125, end - probeStart + 1)`.
- Return `{ Start: probeStart, Count: count }`.

### 6.6 Next() — Backward

- If `backwardLow > backwardHigh`: set `blockStart = originalBlockStart - backwardBest`, `blockEnd = originalBlockStart + 125`, `phase = Forward`, then return Next() (which will issue the first forward read).
- `mid = (backwardLow + backwardHigh) / 2`.
- `chunkStart = originalBlockStart - mid`. If `chunkStart < start`, set `backwardHigh = mid - 1` and recurse Next().
- Return `{ Start: chunkStart, Count: mid }`.

### 6.7 Next() — Forward

- If `blockEnd > end`: set `probeStart = blockEnd`, `phase = Probe`, `hadProbeFailure = false`, return Next().
- Return `{ Start: blockEnd, Count: min(125, end - blockEnd + 1) }`.

### 6.8 Next() — Tail

- If `tailLow > tailHigh`: the confirmed readable region for this block is [blockStart, blockEnd + tailBest). Set `probeStart = blockEnd + tailBest` (or `blockEnd + 125` if tailBest==0). If `probeStart > blockEnd + 1`, set **gap probe** pending: `gapProbeAddr = blockEnd`, `hasGapProbe = true`. Then `phase = Probe`, `hadProbeFailure = false`, return Next().
- `mid = (tailLow + tailHigh) / 2`. If mid < 1 or mid > (end - blockEnd + 1), adjust range and recurse.
- Return `{ Start: blockEnd, Count: mid }`.

### 6.9 OnResult — Probe

- If **lastTaskWasGapProbe** is true: clear lastTaskWasGapProbe and return (no state change; the gap probe result is recorded by the executor only). **Example:** After a block [100,225) and tailBest=0, probeStart=350; a gap probe (225,1) is emitted. If it succeeds, the executor writes that one register to MCAP, but the strategy does not update blockStart, blockEnd, or phase — the next Next() returns the normal 125-block at 350.
- If **success**:
  - If `hadProbeFailure`: set `phase = Backward`, `originalBlockStart = probeStart`, `blockEnd = probeStart + task.Count`. Set `backwardLow = 1`, `backwardHigh = min(125, originalBlockStart - start)`, `backwardBest = 0`.
  - Else: set `phase = Forward`, `blockStart = probeStart`, `blockEnd = probeStart + task.Count`.
- If **failure**: set `hadProbeFailure = true`, `probeStart += 125`.

### 6.10 OnResult — Backward

- `mid = task.Count`.
- If **success**: `backwardBest = mid`, `backwardLow = mid + 1`.
- If **failure**: `backwardHigh = mid - 1` (or 0 if mid was 1).

### 6.11 OnResult — Forward

- If **success**: `blockEnd += task.Count`.
- If **failure**: set `phase = Tail`, `tailLow = 1`, `tailHigh = min(125, end - blockEnd + 1)`, `tailBest = 0`.

### 6.12 OnResult — Tail

- `mid = task.Count`.
- If **success**: `tailBest = mid`, `tailLow = mid + 1`.
- If **failure**: `tailHigh = mid - 1` (or 0 if mid was 1).

### 6.13 Done()

- Return `done`.

### 6.14 Worst-case (no successful reads)

- One read per 125-aligned block: **ceil((end − start + 1) / 125)** (or 1 if range non-empty but smaller than 125).

### 6.15 Example

- start=0, end=400. Probe (0,125) fail, (125,125) fail, (250,125) success. hadProbeFailure=true → Backward. originalBlockStart=250, blockEnd=375. backwardLow=1, backwardHigh=min(125,250)=125. Mid=63, try (187,63); success → backwardBest=63, backwardLow=64. Mid=95, try (155,95); fail → backwardHigh=94. … Eventually blockStart=250−best, blockEnd=375, phase=Forward. Try (375,125); fail → Tail. Binary search for max count at 375 in [1, 25] (since end=400). Then probeStart = 375+tailBest or 500, phase=Probe, continue.
- **Gap-probe path:** Suppose block [100,225), tail search yields tailBest=10 (so only 10 registers readable at blockEnd=225). Then probeStart = 225+10 = 235. Since 235 > 225+1, a **gap probe** is pending at 225. Next() returns (225,1); the executor runs it and, if successful, writes that one register to MCAP. OnResult(Probe) sees lastTaskWasGapProbe and returns without changing state. Next() then returns the normal 125-block at 235.

---

## 7. Algorithm: **boundary**

**Goal:** Given one known successful read (seed), find the **maximal readable interval** containing it with minimal reads. Use after stepped or smart finds a hotspot, or when you have a known-good (start, count) from a prior run.

### 7.1 Parameters

- **SeedStart** (uint16) — start address of the known-good read.
- **SeedCount** (uint16) — register count of the seed (1–125). Required when algo is boundary.
- Plus common config: `StartAddress`, `EndAddress`, `Function`, `Delay` (expansion is clamped to this range).

### 7.2 Phases

- **Seed** — emit the seed task once; on success continue to left expand, on failure set phase = Done.
- **Left expand** — attempted only while **blockStart > StartAddress**. Exponentially try (blockStart − 1, 1), (blockStart − 2, 2), (blockStart − 4, 4), … up to 125. On success, blockStart moves left; on failure, switch to **left binary** with leftLow = task.Start, leftHigh = blockStart. **Clamping (one rule):** if `blockStart - size < StartAddress`, **clamp** the read: issue `(StartAddress, blockStart - StartAddress)` only if that count is **≥ 1**. If clamping would yield Count == 0 (e.g. blockStart == StartAddress), **left expand is complete**; set **leftLow = StartAddress, leftHigh = blockStart**, then transition to **LeftBinary** without emitting a task. **Never emit a task with Count == 0.**
- **Left binary** — binary search for the leftmost readable address; then go to **right expand**. **Invariants:** On entering LeftBinary, `leftHigh` is known readable and `leftLow` is a known non-readable or lower search bound; the true left boundary lies in [leftLow, leftHigh]. **When left expand completed without a failure** (e.g. seed at StartAddress), bounds are leftLow = StartAddress, leftHigh = blockStart; if leftLow ≥ leftHigh, there is no search space — do not emit a task; set phase = RightExpand immediately. Otherwise termination occurs when no address remains between them (leftLow ≥ leftHigh).
- **Right expand** — try (blockEnd, 1), (blockEnd, 2), … up to 125. On success, blockEnd extends right; on failure, switch to **right binary** with rightLow = blockEnd, rightHigh = min(blockEnd+125−1, end, 65535). **Clamping (one rule):** if `blockEnd + size - 1 > EndAddress` or would exceed 65535, **clamp** the read: issue `(blockEnd, EndAddress - blockEnd + 1)` only if that count is **≥ 1**. If clamping would yield Count == 0, **right expand is complete**; set **rightLow = blockEnd**, **rightHigh** = min(blockEnd+125−1, end, 65535), then transition to **RightBinary** without emitting a task. **Never emit a task with Count == 0.**
- **Right binary** — binary search for the rightmost readable address; then **done**. **Invariants:** On entering RightBinary, `rightLow` is the first candidate beyond the currently confirmed readable region and `rightHigh` is the greatest address still possibly readable. **When right expand completed without a failure** (e.g. blockEnd already at EndAddress), if rightLow > rightHigh there is no search space — do not emit a task; set phase = Done immediately. Otherwise termination occurs when rightLow > rightHigh.

### 7.3 State

- `blockStart`, `blockEnd` — current known readable interval [blockStart, blockEnd) (exclusive end). Initially from seed.
- `phase` — Seed | LeftExpand | LeftBinary | RightExpand | RightBinary | Done.
- `seedEmitted` — true after the seed task has been returned.
- Left expand: `leftExpandIdx` into sizes [1,2,4,8,16,32,64,125]. Left binary: `leftLow`, `leftHigh` (invariants above).
- Right expand: `rightExpandIdx`. Right binary: `rightLow`, `rightHigh` (invariants above).

### 7.4 Init

- `blockStart = SeedStart`, `blockEnd = SeedStart + SeedCount`, `start = StartAddress`, `end = EndAddress`.
- `phase = Seed`, `seedEmitted = false`.
- **Seed validation:** The **entire seed read** must lie inside the configured range [StartAddress, EndAddress]. If **SeedStart < StartAddress**, or **SeedStart + SeedCount − 1 > EndAddress**, or SeedCount is invalid (0 or >125), set `phase = Done` immediately (strategy emits no tasks). Reject or treat as done so that the seed task itself is never issued out-of-range.

### 7.5 Next() / OnResult

- **Seed**: Next returns (SeedStart, SeedCount) once; OnResult success → phase = LeftExpand; failure → phase = Done.
- **Left expand**: only while blockStart > StartAddress. For next size in list: if blockStart − size < StartAddress, clamp to (StartAddress, blockStart − StartAddress) only if that count ≥ 1; if count would be 0, left expand complete → set leftLow = StartAddress, leftHigh = blockStart, phase = LeftBinary (no task). Else return (blockStart − size, size) or clamped task. Success → blockStart = task.Start. Failure → phase = LeftBinary, leftLow = task.Start, leftHigh = blockStart.
- **Left binary**: if leftLow ≥ leftHigh (no search space), phase = RightExpand, no task. Else mid = (leftLow+leftHigh)/2, count = blockStart−mid; return (mid, count). Success → blockStart = task.Start, leftHigh = task.Start. Failure → leftLow = task.Start+task.Count. When leftLow ≥ leftHigh, phase = RightExpand.
- **Right expand**: for next size: if read would exceed EndAddress, clamp to (blockEnd, EndAddress − blockEnd + 1) only if that count ≥ 1; if count would be 0, right expand complete → set rightLow = blockEnd, rightHigh = min(blockEnd+125−1, end, 65535), phase = RightBinary (no task). Else return (blockEnd, size) or clamped task. Success → blockEnd = task.Start+task.Count. Failure → phase = RightBinary, rightLow = blockEnd, rightHigh = min(blockEnd+125−1, end, 65535).
- **Right binary**: if rightLow > rightHigh (no search space), phase = Done, no task. Else mid = (rightLow+rightHigh)/2 (use uint32 to avoid overflow), count = mid−blockEnd+1; return (blockEnd, count). Success → blockEnd = task.Start+task.Count, rightLow = task.Start+task.Count. Failure → rightHigh = mid−1 (or task.Start+task.Count−2). When rightLow > rightHigh, phase = Done.

**Binary semantics:** Re-implementations should verify invariants and off-by-one behaviour for: one-register extension (count 1 at boundary), full failure immediately next to seed, and seed touching StartAddress or EndAddress.

### 7.6 Done()

- Return true when phase == Done.

### 7.7 Worst-case

- 1 seed + left expand (up to 8 sizes) + left binary (O(log range)) + right expand (8) + right binary (O(log range)). Depends on range; no fixed formula.

---

## 8. Summary table

| Algo     | Goal                                | Worst-case reads (no hits)    |
|----------|-------------------------------------|-------------------------------|
| safe     | Linear, try sizes 125..1            | rangeLen × 8                  |
| smart    | Divide-and-conquer 125 chunks       | ≈ 2×rangeLen − initialChunks  |
| deep     | Smart + evidence-driven refinement  | smart + up to 500             |
| stepped  | Step positions × 6 probes           | 6 × len(stepPositions)        |
| linear   | 125-blocks + binary tail/back       | ceil(rangeLen/125)            |
| boundary | Expand from seed + binary boundaries| 1 seed + O(expand + log range)|

With `rangeLen = EndAddress − StartAddress + 1`, `initialChunks = ceil(rangeLen/125)`. For **stepped**, worst-case = **6 × len(stepPositions)** (Step ≥ 1; when StepHalfOffset is false, len(stepPositions) = ceil(rangeLen/Step)).

---

## 9. Algorithm selection guidance

- **safe** — Device is fragile; you need predictable behavior; range is small. Conservative fallback.
- **smart** — Best default. Unknown device; medium/large range; best balance of completeness and efficiency.
- **deep** — After smart found interesting regions; you want precise interval edges. Evidence-driven refinement at success/failure boundaries only.
- **stepped** — Range is huge; cheap first pass; triage many devices. Not a final mapper.
- **linear** — Suspect long contiguous maps; PLC-style devices; speed over fragmentation tolerance.
- **boundary** — You have one known-good (start, count); you want the maximal readable interval around it. Use `--seed-start` and `--seed-count`.

---

## 10. Re-implementation checklist

- Implement the **executor** loop and **strategy** interface (Init, Next, OnResult, Done). If StartAddress > EndAddress, strategy is done immediately.
- **ScanTask**: 1 ≤ Count ≤ 125. **ScanResult**: Success == true iff OutcomeType == success.
- For **safe**: candidate list [125,64,32,16,8,4,2,1], linear current, recompute candidates when advancing; **left-boundary probe** state (leftBoundaryProbePending, leftBoundaryProbeAddr); Next() returns pending probe first; Done() is true only when current > end and no probe pending; probe is one-off (does not alter current).
- For **smart**: **priority queue** (min-heap by Count, tie-break by Start), seed with 125-chunks, split failed intervals in half, dedupe with visited set.
- For **deep**: run smart, collect readable and failed intervals; refinement only where a **failed interval intersects [edge−1, edge+1]**; ±8 window, counts [1,2,4,8], cap at 500; Done() in phase 1 returns false (completion detected in Next()).
- For **stepped**: if Step == 0, strategy done immediately (no tasks). Step positions (with **StepHalfOffset**: add start+Step/2+k×Step, dedupe, sort); 6 probes per step (pos±1, pos × count 1,2); use **hasHit** (not hitAddr==0); expansion **strictly clamped** to [StartAddress, EndAddress]; only micro-probes may extend ±1 at edges.
- For **linear**: four-phase state machine (Probe → Backward if late hit, else Forward → Tail → Probe), binary search for backward extent and tail count; **hasGapProbe** + **gapProbeAddr** (not address-as-sentinel); **lastTaskWasGapProbe**: OnResult(Probe) does not update state when true; gap probe is observation-only.
- For **boundary**: seed validation — full seed inside [StartAddress, EndAddress] (else phase=Done). At left/right expand, **clamp when possible**; if clamping would yield Count 0, emit no task and transition to next phase (set leftLow/leftHigh or rightLow/rightHigh as in 7.2/7.5, then LeftBinary or RightBinary). When entering a binary phase with no search space (leftLow ≥ leftHigh or rightLow > rightHigh), emit no task and transition immediately (RightExpand or Done). Left/right binary invariants and termination as specified.

All addresses and counts are 16-bit unsigned; clamp to [0, 65535] and to the configured start/end where specified.

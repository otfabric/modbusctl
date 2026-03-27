# `--format json` field conventions (client commands)

These conventions are a **stable contract** for scripting. Prefer **typed structs** in `internal/types/*_output.go` with explicit `json` tags; avoid `map[string]any` unless unavoidable.

| Concept | JSON fields |
|--------|-------------|
| Transport | `target` (e.g. `tcp://host:502`) |
| Unit | `unit_id` (number) |
| Function / registers | `function`, `start_address`, `register_count` when applicable |
| Repeated rows | `objects`, `blocks`, `units`, etc. as **arrays of objects** |
| Per-unit or row failure | `error` (string; omit or empty when OK) |
| Run-level metadata | Prefer a single pattern: either nested `meta` or consistent top-level keys — pick one per command family and reuse |
| Scan final summary | `ScanSummaryResult`: `target`, `algo`, request/capture counters, `duration`, `mcap_output_path` (live scan progress stays on stderr) |
| Record final summary | `RecordSummaryResult`: `target`, `blocks_recorded`, `iterations`, `mcap_output_path` (per-iteration progress stays on stderr) |

Breaking changes to names or shapes should be called out in release notes.

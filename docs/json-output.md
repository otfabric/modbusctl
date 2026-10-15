# `--format json` field conventions (client commands)

These conventions are a **stable contract** for scripting. Prefer **typed structs** in `internal/types/*_output.go` with explicit `json` tags; avoid `map[string]any` unless unavoidable.

| Concept | JSON fields |
|--------|-------------|
| Transport | `target`: the Modbus URL (e.g. `tcp://host:502`, `udp://host:502`, `rtu:///dev/ttyUSB0`, `ascii://COM3`, `rtuovertcp://host:4001`) |
| Unit | `unit_id` (number) |
| Function / registers | `function`, `start_address`, `register_count` when applicable |
| Repeated rows | `objects`, `blocks`, `units`, etc. as **arrays of objects** |
| Per-unit or row failure | `error` object (`kind`, `code`, `message`); omitted when OK |
| Run-level metadata | Prefer a single pattern: either nested `meta` or consistent top-level keys — pick one per command family and reuse |
| Scan final summary | `ScanSummaryResult`: `target`, `algo`, request/capture counters, `duration`, `capture_output_path` (the MBCAP capture file written) (live scan progress stays on stderr) |
| Write result | `WriteResult`: `target`, `unit_id`, `function` (resolved: 5, 6, 15 or 16), `start_address`, `count`, and either `coils` (booleans) or `registers` (numbers) with `raw_data_hex`; `codec` when one was used |
| Poll sample (stream) | `PollSample`, **one compact JSON object per line** (JSON Lines), written as each poll completes: `poll` (1-based), `timestamp` (RFC 3339, UTC, milliseconds), `unit_id`, `function`, `start_address`, `count`, `address_step` (address distance between consecutive `values`: the codec register width, or 1), `rtt_ms`, `raw_data_hex`, `values` (booleans for functions 1/2, numbers for 3/4, codec-typed values with `codec`), and `error` (`kind`, `code`, `message`) on a failed poll, in which case `values` is empty. The end-of-run summary goes to stderr as text |
| Record final summary | `RecordSummaryResult`: `target`, `blocks_recorded`, `iterations`, `capture_output_path` (the MBCAP capture file written) (per-iteration progress stays on stderr) |

Breaking changes to names or shapes should be called out in release notes.

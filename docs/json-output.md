# `--format json` field conventions

These conventions are a **stable contract** for scripting. Prefer **typed structs** in `internal/types/*_output.go` with explicit `json` tags; avoid `map[string]any` unless unavoidable.

| Concept | JSON fields |
|--------|-------------|
| Transport | `target`: the Modbus URL (e.g. `tcp://host:502`, `udp://host:502`, `rtu:///dev/ttyUSB0`, `ascii://COM3`, `rtuovertcp://host:4001`) |
| Unit | `unit_id` (number) |
| Function / registers | `function`, `start_address`, `register_count` when applicable |
| Repeated rows | `objects`, `blocks`, `units`, etc. as **arrays of objects** |
| Per-unit or row failure | `error` object (`kind`, `code`, `message`); omitted when OK |
| Fatal error | one object `{"error": {"kind", "code", "message"}}` on stdout, nothing else, also when the invocation itself is wrong (an unknown flag, a missing argument, a missing `--yes`, a malformed environment variable) and JSON was asked for with `--format json` or `MODBUSCTL_FORMAT=json`; see [exitcodes.md](exitcodes.md). Logs and progress stay on stderr, also of `client poll`, whose earlier records remain on stdout before the error object |
| Run-level metadata | Prefer a single pattern: either nested `meta` or consistent top-level keys — pick one per command family and reuse |
| Scan final summary | `ScanSummaryResult`: `target`, `algo`, request/capture counters, `duration`, `capture_output_path` (the MBCAP capture file written) (live scan progress stays on stderr) |
| Write result | `WriteResult`: `target`, `unit_id`, `function` (resolved: 5, 6, 15 or 16), `start_address`, `count`, and either `coils` (booleans) or `registers` (numbers) with `raw_data_hex`; `codec` when one was used |
| Poll sample (stream) | `PollSample`, **one compact JSON object per line** (JSON Lines), written as each poll completes: `poll` (1-based), `timestamp` (RFC 3339, UTC, milliseconds), `unit_id`, `function`, `start_address`, `count`, `address_step` (address distance between consecutive `values`: the codec register width, or 1), `rtt_ms`, `raw_data_hex`, `values` (booleans for functions 1/2, numbers for 3/4, codec-typed values with `codec`), and `error` (`kind`, `code`, `message`) on a failed poll, in which case `values` is empty. The end-of-run summary goes to stderr as text |
| Record final summary | `RecordSummaryResult`: `target`, `blocks_recorded`, `iterations`, `capture_output_path` (the MBCAP capture file written) (per-iteration progress stays on stderr) |

**Strings from devices and files** (FC43 identification objects, codec-decoded strings, device profile names): JSON carries the exact value (encoding/json escapes control characters). Text and table output, and the `error:` line on stderr, write control characters, Unicode bidirectional controls and bytes that are not UTF-8 as Go escapes (`\x1b`, `\n`, `\u202e`, `\xff`), so a device cannot drive the terminal or forge output lines.

## Fields per command

The JSON output types live in `internal/types`. Fields marked *optional* are left out when empty.

| Command | Type | Fields |
|---------|------|--------|
| `client read` | `ReadResult` | `target`, `unit_id`, `function`, `start_address`, `register_count`, `raw_byte_count`, `raw_data_hex`, `capture_output_path` |
| `client decode` | `DecodeResult` | `target`, `unit_id`, `function`, `start_address`, `register_count`, `codec`, `family`, `value_kind`, `raw_data_hex`, `values` |
| `client codecs` | `CodecList` | `codecs`: an array of `id`, `name`, `family`, `value_kind`, `registers` (register width), `bytes` |
| `client write` | `WriteResult` | see the table above |
| `client poll` | `PollSample` | see the table above. The end-of-run `PollSummary` (`target`, `polls`, `succeeded`, `failed`, `duration`, `interrupted`) goes to stderr as text |
| `client scan` | `ScanSummaryResult` | `target`, `summary` (per read request, optional), `algo`, `total_requests`, `success_count`, `fail_count`, `exception_count`, `timeout_count`, `transport_error_count`, `blocks_captured`, `registers_captured`, `avg_response_ms` (optional), `duration`, `capture_output_path` |
| `client record` | `RecordSummaryResult` | see the table above |
| `client identify` | `IdentifyResult` | `target`, `summary` (optional), `units`: per unit `unit_id`, `error` (optional), and, all optional, `category`, `conformity_level`, `more_follows`, `next_object_id`, `objects` (`id`, `value`, `description` (optional)) and `report_server_id` (`error`, `data_hex`, `run_indicator_on`, all optional) |
| `client reportserverid` | `ReportServerIDResult` | `target`, `summary` (optional), `units`: per unit `unit_id`, and, all optional, `error`, `data_hex`, `run_indicator_on` |
| `client fingerprint` | `FingerprintResult` | `target`, `summary` (optional), `units`: per unit `unit_id`, and, all optional, `error`, `supported_read_functions`, `probe_interrupted` |
| `client diagnostic` | `DiagnosticResult` | `target`, `unit_id`, `sub_function` (name), `sub_function_hex` (the sub-function code), `data_hex` |
| `client sunspec detect` | `SunSpecDetectOutput` | `target`, `unit_id`, `regtype`, `detected`, `base_address`, `attempts` (optional): per probed base `index` (1-based), `base_address`, `matched`, `result` (`matched SunS`, `no marker` or the error) |
| `client sunspec models` | `SunSpecModelsOutput` | `target`, `base`, `models`, `not_detected` (optional) |
| `client sunspec map` | `SunSpecMapOutput` | `target`, `base`, `marker_regs` (the two marker registers, `base-base+1`), `models`, `not_detected` (optional) |
| `client sunspec probe` | `SunSpecProbeOutput` | `target`, `unit_id`, `modbus` (`fc03`, `fc04`, `fc43`: function supported; `fc03_error`, `fc04_error`, `fc43_error`: optional, when the probe itself failed), `sunspec` (`detected`, `base_address`, `models_found`, `end_model`, and the optional `detection_error` and `model_headers_error`) |
| `discover` | `DiscoverOutput` | `port`, `interface` (optional), `subnets` (optional), `devices`: per device `ip`, `port`, `mac`, `interface`. With `--output`, the file holds the `devices` array alone |
| `version` | `buildinfo.Info` | `version --json` or `MODBUSCTL_FORMAT=json`: one line, keys in this order, all present: `name`, `version` (no leading `v`; `dev` for a development build), `tag` (`""` when the build has no tag), `commit` (full SHA, `-dirty` for a modified tree, or `unknown`), `built` (RFC 3339 UTC or `unknown`), `libraries` (the OT Fabric libraries the binary is built with: `name`, `version`; `[]` when none), `go`, `platform` (`goos/goarch`) |
| `capture extract` | `AddressBlock` | the output file is an array of `start_address`, `register_count` |

A SunSpec model header (in `models`) has `id`, `length`, `start_address`, `end_address`, `next_address` and `is_end_model`. A `summary` (`ResultSummary`) has `requested`, `succeeded` and `failed`.

## Device profile (input of `capture decode --profile`)

```json
{
  "protocolData": {
    "registers": [
      {"controlledPropertyId": "power", "valueScaleFactor": 0.1, "start": 100, "size": 2, "format": ">i"}
    ]
  }
}
```

| Field | Meaning |
|-------|---------|
| `protocolData` | holds `registers`, the register definitions |
| `controlledPropertyId` | the name of the value in the output |
| `start` | the first register address |
| `size` | the number of registers |
| `format` | the byte order and type, Python `struct` style: `>` big-endian or `<` little-endian, then `h`/`H` (16-bit signed/unsigned), `i`/`I` (32-bit), `q`/`Q` (64-bit), `f` (float32), `d` (float64); and `>B` (one byte) |
| `valueScaleFactor` | the decoded value is multiplied by it |

## Error object

Used for a fatal error (`{"error": {…}}`) and for a row that failed.

| Field | Meaning |
|-------|---------|
| `kind` | `usage`, `invalid_input`, `transport`, `timeout`, `protocol`, `modbus_exception`, `output`, `internal` or `interrupted` |
| `code` | stable code; see [exitcodes.md](exitcodes.md) |
| `message` | text for people; do not parse it |

```json
{
  "error": {
    "kind": "usage",
    "code": "confirmation_required",
    "message": "client write changes the state of the device: confirm with --yes (nothing was sent)"
  }
}
```

Breaking changes to names or shapes should be called out in release notes.

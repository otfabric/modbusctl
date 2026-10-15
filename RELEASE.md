# modbusctl Releases

## v3.1.0

**Date:** 2026-10-15  
**Previous release:** v3.0.0

### Summary

Minor release: **`server static`** and **`server replay`** now answer every function a capture holds, not only FC01–04, FC07, FC11, FC12 and FC43 device identification. Adopts **go-modbus v1.7.0**, whose server dispatches every function code. No changes to flags or JSON output.

### Added

- **Every recorded function is served:** FC08 Diagnostics, FC17 Report Server ID, FC20 Read File Record, FC24 Read FIFO Queue, FC43 MEI types other than device identification and vendor function codes are answered with the response recorded for exactly that request, byte for byte. A request the capture does not hold is answered according to `--missing`, as for the other functions.

### Changed

- **FC07, FC11 and FC12** are served the same way: the recorded response PDU is returned as it is instead of being decoded and re-encoded.
- **Writes** through FC21 Write File Record, FC22 Mask Write Register and FC23 Read/Write Multiple Registers are refused with Illegal Function, like the other writes.
- **go-modbus:** Requires **`github.com/otfabric/go-modbus` v1.7.0** (was v1.6.1).

## v3.0.0

**Date:** 2026-10-14  
**Previous release:** v2.4.1

### Summary

Major release: captures are now **MBCAP** files, the format of [**go-modbus-capture**](https://github.com/otfabric/go-modbus-capture), instead of MCAP v1. A capture stores the **raw request and response PDUs** of every answered read with the time the response arrived, for several devices and acquisition streams in one file. **`server static`** and **`server replay`** are rebuilt on it: they serve every target of a capture on its own unit id, coils, discrete inputs and registers per address, FC07/FC11/FC12 and FC43 device identification, and **`server replay`** plays the recording at any speed. **MCAP v1 files are no longer read.** Also adopts **go-modbus v1.6.1**.

### Breaking changes

- **Capture format:** **`client read`**, **`client scan`** and **`client record`** write MBCAP (`.mbcap`) instead of MCAP v1 (`.mcap`); automatic file names are now `modbusctl_<kind>_<UTC time>.mbcap`. **`server`** and **`mcap`** commands read MBCAP in any of its encodings (`.mbcap`, `.mbcap.jsonl`, `.mbcap.csv`) and reject an MCAP v1 file with a message saying so.
- **`server static`:** `--function` is gone: a capture may hold several function codes, and all of them are served. `--unit` now defaults to **0**, meaning each target answers on the unit id it was recorded with; `--unit` serves a single target (chosen with the new `--target`) on another unit id.
- **`server replay`:** `--function` and `--interval` are gone. Timing follows the recorded sample times; `--speed` scales it (1 = original, 10 = ten times faster, 0.1 = ten times slower). `--loops 0` still loops forever; `--loops N` now plays the capture N times and then keeps serving the last values.
- **`mcap convert --format`:** `csv` now writes the lossless MBCAP long CSV (one row per record, raw PDUs in hex); `json` is replaced by **`jsonl`** (MBCAP JSON Lines); new **`mbcap`** and **`wide-csv`** (raw values per address over time).
- **`mcap info`** prints an MBCAP summary (targets, streams, sample counts, functions, span, poll interval) instead of the v1 header and iteration statistics.
- **Answers for data a capture does not hold** are now chosen with `--missing`; the default (exception) matches v2 for addresses: Illegal Data Address; a function never recorded now answers Illegal Function on both servers.

### Added

- **`server static --at`** serves the values at a moment of the capture instead of its last values; **`server replay --from`** starts part-way in.
- **`--missing exception|zero|silence`** and (replay) **`--before-first hold|absent`** on both servers.
- **`--max-clients`** on both servers (default 10, as before): the number of concurrent client connections served; further ones are closed.
- **Server summary:** on shutdown both servers print what they served (`📊 Served N requests: …`), from go-modbus server metrics: answers, exceptions, silences, and requests for data the capture does not hold. With `--debug` they also name each such request.
- **Error codes** for capture files (exit 3): `capture_legacy_format` (an MCAP v1 file), `capture_selection_ambiguous` and `capture_selection_empty` (a `--target`/`--function` selection that matches several or no targets and functions).
- **`--target`** on both servers, and on **`mcap decode`**, **`extract`**, **`strings`** and **`frequencies`** together with **`--function`**, to pick one target and function out of a capture holding several.
- **`mcap verify`** checks a capture's block checksums, structure and the rules of the format.
- **`--record-exceptions`** on **`client record`** and **`client scan`** also records exception responses (for a scan: where the device refuses).
- Serial captures keep the serial settings of the device (baud, parity, data and stop bits); every capture keeps the connection URL in the target's `url` tag.

### Changed

- **Recording:** samples come from the go-modbus **PDU tap**, so they are the exact bytes on the wire. A read that is retried (busy device, reconnect) leaves one sample, of the attempt that succeeded; a failed read leaves none. The file is flushed every 10 seconds: an interrupted recording is readable up to the last flush.
- **`server static`** serves several coils or discrete inputs in one request correctly (v2 answered only the first one right).
- **Writes** to either server are refused with Illegal Function: a replayed device is read-only.
- **go-modbus:** Requires **`github.com/otfabric/go-modbus` v1.6.1** (was v1.3.0). Adds **`github.com/otfabric/go-modbus-capture` v0.0.1**.

### Migration

- Re-record devices with modbusctl 3. Old `.mcap` files can be kept and inspected with modbusctl 2.x; nothing converts them.
- Scripts that passed `--function` to **`server static`/`replay`**: drop it. Scripts that passed `--interval` to **`server replay`**: use `--speed` (original timing is the default).
- Scripts that ran **`mcap convert --format json`**: use `--format jsonl` (one JSON object per line: header, target, stream, sample and end records with hex PDUs).
- The JSON summaries of **`client read`**, **`scan`** and **`record`** keep the key `mcap_output_path`; it names the `.mbcap` file.

## v2.4.1

**Date:** 2026-10-11  
**Previous release:** v2.4.0

### Summary

Patch release: adopts **go-modbus v1.3.0**, which hardens the client against lost, slow and out-of-step connections. No changes to commands, flags or JSON output.

### Changes

- **go-modbus:** Requires **`github.com/otfabric/go-modbus` v1.3.0** (was v1.2.1).
- **No stale answers after a timeout (TCP/TLS):** After a request times out or a response is malformed, the connection is replaced before the next request. A late response to the timed-out request can no longer be taken for the answer to the next one, which matters most for **`client poll`**, **`client record`** and **`client scan`** against slow devices.
- **Reconnects by itself:** A long-running **`client poll`** or **`client record`** whose connection drops fails that one sample and continues on a new connection with the next.
- **Ctrl-C and `--timeout` take effect immediately:** A request in flight is interrupted instead of running to its timeout.

## v2.4.0

**Date:** 2026-10-09  
**Previous release:** v2.3.0

### Summary

Closes the capability gap with classic poll-and-write tools. modbusctl now speaks **Modbus RTU and ASCII on serial lines**, **Modbus UDP**, and RTU/ASCII framing through serial-to-Ethernet bridges; it can **write coils and holding registers** (**`client write`**) and **poll continuously at an interval** without recording (**`client poll`**). Every existing client command works over the new transports. No breaking changes to flags or JSON output.

### Highlights

- **Transports (all client commands):** **`--url`** now accepts **`rtu://<device>`**, **`ascii://<device>`**, **`udp://host:port`**, **`rtuovertcp://`**, **`rtuoverudp://`** and **`asciiovertcp://`** in addition to **`tcp://`**. Serial devices are written as `rtu:///dev/ttyUSB0` (Linux/macOS) or `rtu://COM3` (Windows).
- **Serial settings:** New **`--baud`**, **`--data-bits`**, **`--parity`** (`none`/`even`/`odd`) and **`--stop-bits`** flags on every client command, with **`MODBUSCTL_BAUD`**, **`MODBUSCTL_DATA_BITS`**, **`MODBUSCTL_PARITY`** and **`MODBUSCTL_STOP_BITS`**. Defaults: 19200 baud, 8 data bits for RTU and 7 for ASCII, no parity, 2 stop bits without parity or 1 with. They are ignored for network URLs.
- **`client write` (new):** Writes coils (**FC05**, **FC15**) and holding registers (**FC06**, **FC16**). **`--values`** takes one value or a comma-separated list; with **`--function`** omitted, one register uses FC06 and several use FC16. **`--codec`** encodes typed values (floats, 32/64-bit integers, strings, IP addresses, timestamps) with the same codec registry as **`client decode`**. **`--start`** is mandatory, everything is validated before anything is sent, and a write is sent exactly once (never retried).
- **`client poll` (new):** Reads the same block of coils, discrete inputs or registers every **`--interval`** ms and prints each sample as it is read — one line (**text**), one JSON object per line (**json**) or one row (**table**) — until **`--polls`** samples or Ctrl-C. Optional **`--codec`** decoding. Failed polls are reported in-stream and polling continues (reconnecting when needed); exit code **7** when any poll failed. Nothing is written to disk; **`client record`** remains the MCAP capture command.
- **Serial buses:** On `rtu://` and `ascii://`, **`identify`** and **`reportserverid`** ignore **`--parallel`** and query units one after the other, since a serial line carries one request at a time.
- **MCAP:** Captures made over a serial line store the device path in the header **`ip`** field with port **0**. The format is unchanged.
- **Completion:** Tab completion for **`--parity`** and for **`client write --function`** (5, 6, 15, 16).
- **go-modbus:** Requires **`github.com/otfabric/go-modbus` v1.2.0**, which adds the Modbus ASCII transport and corrects function probing.

### Fixes

- **Silent empty result after a dropped connection:** `client read`, `decode` and `scan` could report success with no data when the connection was reset during the request: the reconnect succeeded and its result overwrote the read error. The read error is now reported (and counted as a transport failure by `scan`).
- **`client scan --algo safe` (the default) lost registers:** After finding a block that followed unreadable addresses, the scan skipped one register (for example the last register of a short block), and could read one register below `--start`. Both are fixed.
- **`client scan` with `--end 65535` (the default):** `--algo smart` and `--algo deep` never finished building their work list and ran out of memory, and `--algo safe` could wrap around instead of finishing. Position arithmetic no longer overflows at the top of the address space.
- **`client scan --algo linear`:** No longer loops forever when the range reaches address 65535, and no longer issues zero-register reads for the full range 0–65535.
- **`client scan --algo boundary`:** Finds the left edge of blocks of any size (it used to stop after the first failed read), never reads more than 125 registers at once, and handles blocks and seeds that reach address 65535.
- **`client fingerprint` and `client sunspec probe`:** A function is reported as supported only when the device implements it. Previously any exception counted, so a device answering *Illegal Function* — or a gateway answering for an absent unit — appeared to support every probed function; and a device that correctly echoed FC08 was reported as not supporting diagnostics. (Fixed in go-modbus v1.2.0.)
- **MCAP files cut short:** A capture truncated exactly between two fields of a record was read as complete, silently dropping the damaged record. Every `mcap` command now reports it as an unexpected end of file. A record too large for the format is rejected instead of corrupting the file.
- **`server static` / `server replay` with coils or discrete inputs:** A request for more than one coil returned wrong values for every coil after the first. Multi-coil reads now return the recorded bits.
- **`server replay`:** Fixed a data race between the startup message and the replay loop.
- **`client sunspec` commands:** `--timeout` now bounds every read. Previously a device that did not answer blocked for the whole operation budget (10 s or more) on the first base address.
- **`client identify` / `client reportserverid` with several units:** Ctrl-C now stops at the next unit. Previously every remaining unit still waited for its full timeout.
- **Error classification:** A connection closed by the device is reported as a transport error (exit code 4), not an internal error (10).
- **`--format table`:** The rule line under the header no longer ends in trailing spaces.
- **Validation message:** An `--output` path whose parent is a file reported `%!w(<nil>)`; it now says the output directory is not a directory.
- **Error text:** Connection failures are reported as **`connection error: …`** instead of `TCP connection error: …`, since the transport may be a serial port. The JSON **`kind`** (`transport`) and **`code`** (`transport_connect_failed`) are unchanged.
- **Documentation:** `float32/layout:1234` was described as big-endian in examples; it is little-endian. Examples now use **`float32/layout:4321`** (big-endian). `client read` reads **`MODBUSCTL_ADDRESS`** (not `MODBUSCTL_START`) for `--start`; the help example and README now say so.

### Quality

- **Coverage:** Test coverage raised from 51% to **96%** of the whole module, with a floor of 85% enforced by **`make coverage`** (`COVERAGE_MIN`) and a Codecov project target; CI now tests the `cmd` packages too. A **Code Coverage** badge replaces the retired Go Report Card badge.
- **Hardware-free tests:** New **`internal/mbtest`** in-memory Modbus device served over TCP, UDP, RTU/ASCII over TCP, and RTU/ASCII on a pseudo-terminal, so serial transports are exercised end to end in CI.
- **Makefile:** Sets **`GOWORK=off`** so a parent `go.work` cannot interfere.

### Upgrading

- **Scripts matching error text:** Replace matches on `TCP connection error` with the JSON `code` (`transport_connect_failed`) or the exit code (4).
- **Completion users:** Regenerate and re-source your shell completion script to pick up the new commands and flags.
- **Float examples:** If you copied `float32/layout:1234` from earlier documentation expecting big-endian values, switch to `float32/layout:4321`. Codec behaviour itself has not changed.

### Release workflow

- Unchanged from v2.3.0: reusable **`go-binary-release`** workflow and link-time version metadata.

---

## v2.3.0

**Date:** 2026-07-06  
**Previous release:** v2.2.4

### Summary

Adds typed **codec decoding** to the client: a new **`client decode`** command reads holding/input registers and decodes them into typed values (integers, floats, strings, BCD, time, IP, MAC, …) using the **`github.com/otfabric/go-modbus/codec`** registry, plus a **`client codecs`** command to discover available codecs. As part of this, the ad-hoc **`--ascii`** and **`--byteswap`** flags are **removed** from **`client read`**, which now focuses on raw capture and MCAP output. This is a **breaking change** for anyone using those two read flags.

### Highlights

- **`client decode` (new):** Reads holding (**FC3**) or input (**FC4**) registers and decodes them with a single **`--codec`** (e.g. `float32/layout:1234`, `ascii/registers:8`, `uint32/layout:4321`, `ip_addr`, `datetime_iec870_utc`). The codec sets the per-value register width; omit **`--count`** to decode one value, or pass a multiple of the codec width to decode an **array** of values. Output honors **`--format text|json|table`**; JSON emits a typed **`values`** array. No MCAP side effect—decode is a read-and-interpret command.
- **`client codecs` (new):** Lists every registered codec (**id**, **name**, **family**, **value kind**, **registers**, **bytes**), with optional **`--family`**, **`--value-kind`**, and **`--registers`** filters and **`--format text|json|table`**. Mirrors the codec catalog exposed by the Modbus simulator server.
- **Codecs source:** The codec registry ships inside the already-depended-on **`github.com/otfabric/go-modbus`** (`v1.1.0`) under the **`codec`** subpackage—no new dependency and no version bump were required.
- **Completion:** New **`--codec`** flag supports tab completion (registered codec IDs); **`--function`** completion is reused on `decode`.
- **Config / env:** New **`MODBUSCTL_CODEC`** environment variable maps to **`--codec`**; **`MODBUSCTL_COUNT`** gains decode semantics (a multiple of the codec width; `0` = one value).
- **`client read` (breaking):** Removed **`--ascii`** / **`MODBUSCTL_ASCII`** and **`--byteswap`** / **`MODBUSCTL_BYTESWAP`**. The `read` JSON DTO drops **`pre_swap_hex`**, **`bytes_swapped`**, and **`ascii_decoded`**; `read` still captures raw bytes (**`raw_data_hex`**) and writes MCAP.

### Upgrading

- **Replace `read --ascii`:** Use **`client decode --codec ascii/registers:N`** (or `ascii_fixed` / `ascii_reverse` / `utf16be` / `utf16le` variants) for text decoding.
- **Replace `read --byteswap`:** Use a layout-aware numeric codec via **`client decode`** (e.g. **`--codec uint32/layout:2143`**) instead of swapping bytes on a raw read.
- **Scripts parsing `read --format json`:** Stop relying on **`pre_swap_hex`**, **`bytes_swapped`**, and **`ascii_decoded`**; move typed interpretation to **`client decode`** and read its **`values`** array.
- **Completion users:** Regenerate and re-source your shell completion script so the new **`decode`** / **`codecs`** commands and **`--codec`** candidates are available.

### Release workflow

- Unchanged from v2.2.4: reusable **`go-binary-release`** workflow and link-time version metadata.

---

## v2.2.4

**Date:** 2026-03-28  
**Previous release:** v2.2.3

### Summary

Maintenance release: internal package boundaries, validation and tests, and Makefile/CI alignment with full-module typecheck. No intentional user-facing CLI or output-format breaks.

### Highlights

- **Packages:** MCAP binary codec, load, and CSV/JSON/blocks exports live in **`internal/mcap`**; client stdout (**text/json/table**) and MCAP-derived **strings** / **frequencies** helpers stay in **`internal/format`**. Command wiring and formatted runs use **`internal/runner`**; typed CLI errors and exit mapping use **`internal/errs`**.
- **Config & discover:** Modbus URL and SunSpec base parsing are split into focused **`internal/config`** helpers (with tests). **Discover** adds **`--force-large-scan`** and **`MODBUSCTL_DISCOVER_FORCE_LARGE`** to allow probing beyond the default unique-host safety cap.
- **Modbus & MCAP:** Capture path resolution, cancel-aware sleeps, clearer layered address validation, multi-unit pooling where applicable; MCAP export paths share load/stitch/sort; fingerprint **partial** vs **failed** outcomes and related types are clearer.
- **Server:** Static handler FC gating and replay comments tightened.
- **Tooling:** **`make compile`** (`go build ./...`) feeds **`lint`** / **`lint-ci`**; **`check`** runs fmt, lint, lint-ci, vet, test, and coverage; **`check-legacy`** helps catch duplicate root packages. **golangci-lint** uses **`issues.new: false`** and **`run.tests: true`** so the linter sees the same typecheck surface as a full build (see `.golangci.yml`).
- **Tests & docs:** More unit tests across config, modbus, types, format/mcap, and CLI fatals; exit codes remain documented in **`docs/exitcodes.md`**.

### Upgrading

- Same CLI and **`--format`** contract as v2.2.3 unless you rely on discover host-count limits—in which case use **`--force-large-scan`** when you intentionally need a larger probe.

---

## v2.2.3

**Date:** 2026-03-27  
**Previous release:** v2.2.2

### Summary

This release standardizes **client command output** behind a shared **`--format`** flag (**text**, **json**, **table**) and stable JSON DTOs, separates **scan/record** progress (**stderr**) from the final summary (**stdout**), migrates **SunSpec** off standalone **`--json`** to **`--format json`**, and improves **shell completion** for enum-like flags via a small generic helper in **`internal/cli`**.

### Highlights

- **Client output layer:** Results for **identify**, **reportserverid**, **read**, **fingerprint**, **diagnostic**, **scan**, **record**, and **client sunspec** are produced through **`internal/format`** (**`format.Write`**) with typed payloads in **`internal/types`** (`*_output.go`). **`internal/modbus`** collects data and performs MCAP side effects; it does not print final user-facing results to stdout.
- **`--format` and env:** **`MODBUSCTL_OUTPUT_FORMAT`** / **`--format`** on client commands accept **text** (default), **json**, or **table**. This is **distinct** from **`mcap convert`**, which still uses **`--format csv|json`** and **`MODBUSCTL_FORMAT`**.
- **JSON contract:** Field naming conventions for scripts are documented in **`docs/json-output.md`**; **`--format json`** is treated as a stable scripting surface (discipline in release notes for intentional changes).
- **Scan / record:** Live progress, debug lines, and worst-case hints go to **stderr** only; **stdout** emits **one** formatted summary after the run—important for **`--format json`** pipelines.
- **SunSpec:** Subcommands use the same **`--format`** mechanism. A **hidden** parent **`--json`** remains as a deprecated alias mapping to **`--format json`**.
- **Shell completion:** Generic **`cli.RegisterEnumFlagCompletion`** / **`RegisterEnumFlagCompletionWithDescriptions`**; canonical value lists live with their domains (e.g. **`format.Values()`** for client stdout formats, **`config.ScanAlgorithms()`**, **`ConvertFormats()`**, etc.). **`--format`** is registered next to flags on each command (no deferred command-tree walk).
- **Tests:** Golden-style checks for representative text output and JSON structure; config/format completion maps stay aligned with allowed values via small tests.
- **Errors & exits:** Root uses **`ExecuteContext`**, typed **`internal/errs`** failures, **`internal/runner`** for **`AttachOutputFormat` / `RunFormattedWithOutputFormat`**, and JSON fatals buffered to stdout. Exit codes are documented in **`docs/exitcodes.md`**. Embedded unit errors on **identify** / **reportserverid** / **fingerprint** are structured **`ErrorInfo`** in JSON; aggregate text/JSON payloads include a **`summary`** block with requested / succeeded / failed counts. **`ExitPartial` (7)** applies when the formatted result was written successfully but **any** requested unit (or scan request) failed at the Modbus/transport layer—**identify**, **reportserverid**, **fingerprint**, and **scan** (see **`docs/exitcodes.md`**).

### Upgrading

- **Scripts parsing client stdout:** Prefer **`--format json`** and the documented JSON fields; default **text** output is intended to stay close to prior UX where practical.
- **SunSpec:** Replace **`--json`** with **`--format json`**; stop relying on **`MODBUSCTL_JSON`** (removed in favor of **`MODBUSCTL_OUTPUT_FORMAT`**).
- **Scan/record automation:** If you previously assumed progress on stdout, capture **stderr** for logs and **stdout** for the final summary (or use **`--format json`** on stdout only).
- **Completion users:** Regenerate and re-source your shell completion script after upgrading so new **`--format`** candidates and descriptions are picked up.

### Release workflow

- Unchanged from v2.2.2: reusable **`go-binary-release`** workflow and link-time version metadata.

---

## v2.2.2

**Date:** 2026-03-23  
**Previous release:** v2.2.1

### Summary

This release migrates the Modbus stack to **`github.com/otfabric/go-modbus`**, improves release and local builds (including **Windows** artifacts), and standardizes **version metadata** via link-time flags. Behavior of the CLI should remain the same for typical use; see **Upgrading** if you embed scripts or build from source.

### Highlights

- **Modbus library:** Switched from `github.com/otfabric/modbus` to **`github.com/otfabric/go-modbus`** (SunSpec helpers live in `go-modbus/sunspec`). Device identification, Report Server ID, register I/O, and server APIs were updated to match the new module.
- **Client defaults:** Outbound clients now set **`DialTimeout`**, use **`NopLogger`** (or **`NewStdLogger`** when `--debug` is set on read/scan/record), and run **`ValidateConfig`** before connecting.
- **GitHub releases:** Published archives now include **Windows** builds: `modbusctl-<version>-windows-amd64.zip` and `modbusctl-<version>-windows-arm64.zip` (each contains the corresponding `.exe`). Linux and Darwin remain `.tar.gz` as before.
- **Version output:** `modbusctl version` prints **version**, **tag**, **commit**, and **build date**, injected at link time (`-X main.version=…` etc.). The **`version.txt`** file is no longer used for versioning.
- **Local builds:** `make build` / `make build-all` write the binary to **`bin/modbusctl`** (not the repository root). Docker image build copies from that path.

### Release workflow

- Releases are driven by the reusable **`go-binary-release`** workflow (see `.github/workflows/build-release.yml`), with ldflags aligned to the `main.*` variables above.

### Upgrading

- **Prebuilt binaries:** Download the same platform archive as before; add Windows zips if you deploy on Windows.
- **From source:** Use `make build` and run `./bin/modbusctl`, or adjust any scripts that assumed `./modbusctl` in the repo root.
- **Go modules:** Run `go mod tidy` after pulling; direct importers of the old `otfabric/modbus` path should depend on **`otfabric/go-modbus`** instead.

---

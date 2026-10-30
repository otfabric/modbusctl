# modbusctl Releases

## v3.4.0

**Date:** 2026-10-30
**Previous release:** v3.3.0

### Dependencies

- `go-modbus` v1.7.2 → v1.10.0, `golang.org/x/net` → v0.62.0 (govulncheck). Requires Go 1.26, as that `x/net` does.

---

## v3.3.0

**Date:** 2026-10-30
**Previous release:** v3.2.1

### Summary

**Breaking for scripts**, three times: the exit codes follow the OT Fabric organisation table (ADR-0006 of otfabric/engineering), commands that change a device need `--yes`, and in JSON mode every fatal error is one JSON object on stdout.

### `--yes` on commands that change a device

`client write` and the `client diagnostic` sub-functions that change the state of the device now need `--yes`. Without it they exit 2 (kind `usage`, new code `confirmation_required`) and send nothing. **Add `--yes` to scripts that call these commands.** The flag has no environment variable. Everything else stays read-only and needs no confirmation.

| Command | Needs `--yes` |
|---|---|
| `client write` | always |
| `client diagnostic --sub-function` | `restartcommunications`, `changeasciiinputdelimiter`, `forcelistenonlymode`, `clearcountersanddiagnosticreg`, `clearoverruncounterandflag` |

### JSON errors

With `--format json` or `MODBUSCTL_FORMAT=json`, a fatal error is exactly one `{"error":{"kind","code","message"}}` object on stdout, as in bacnetctl, with nothing on stderr unless `--debug` is set. Cases that were text on stderr before:

- Invocations rejected by the command line parser: unknown command or flag, flag value of the wrong type, missing required flag, wrong arguments (kind `usage`, code `invalid_usage`).
- `discover` input rejected before the format was read (kind `invalid_input`).
- A malformed `MODBUSCTL_*` variable (kind `invalid_input`, exit 3). It was a Go panic with a stack trace and exit 2; it is now one `error:` line in text mode too.

Two related changes:

- An unknown subcommand of a group (`client nope`, `server nope`, `client sunspec nope`) exits 2. It printed the help of the group to stdout and exited 0.
- `server static` and `server replay` print their progress to stderr, not stdout. Scripts that read those lines from stdout must read stderr.

### Exit codes

| Case | Old | New | JSON error |
|---|---|---|---|
| Interrupted by SIGINT or SIGTERM before a result (`context_canceled`) | 10 | 130 | kind `interrupted`, was `internal`; also on an embedded `error` of a result, such as a unit of `fingerprint`, whose exit code stays 7 |
| Invocation rejected by the command line parser: unknown command or flag, flag value of the wrong type, missing required flag, wrong arguments | 10 | 2 | kind `usage`, new code `invalid_usage`: the object is new, see [JSON errors](#json-errors) |
| Input file not usable: `capture_load_failed`, `input_file_open_failed`, `input_decode_failed` | 10 | 3 | kind `invalid_input`, was `output` |

Every other exit code is unchanged. See [docs/exitcodes.md](docs/exitcodes.md).

### Environment

`MODBUSCTL_FORMAT` is now the canonical name of the format variable; `MODBUSCTL_OUTPUT_FORMAT` stays accepted, and `MODBUSCTL_FORMAT` wins when both are set. (`MODBUSCTL_FORMAT` belonged to `mcap convert` until v3.1.0; that command is gone.)

---

## v3.2.1

**Date:** 2026-10-29
**Previous release:** v3.2.0

### Summary

Dependency update for Modbus RTU and ASCII over serial lines: go-serial v0.1.8 fixes two POSIX serial I/O bugs. No command or flag changes.

### Dependencies

- go-modbus v1.7.1 → v1.7.2, go-serial v0.1.6 → v0.1.8.

### Fixes (from go-serial v0.1.8)

- `Write` no longer loses data when the serial port's output buffer is
  full. It used to return a short count with a nil error (or `n == -1`
  with `EAGAIN`); it now blocks until every byte is written.
- `Read` works when the port's file descriptor is 1024 or higher. It used
  to return a spurious `io.EOF` at once.

### Other

- Documentation: exit code 5, the `MODBUSCTL_*` table and the serial defaults match the code; security contact is security@otfabric.io.

---

## v3.2.0

**Date:** unreleased
**Previous release:** v3.1.0

### Summary

Minor release: **`server static`** and **`server replay`** serve **Modbus/TCP Security** (`tcp+tls`) with client certificates required, given **`--tls-cert`**, **`--tls-key`** and **`--tls-ca`**. They served plain Modbus TCP only. Without the flags they serve plain Modbus TCP as before.

### Added

- **Modbus/TCP Security on both servers: `--tls-cert`, `--tls-key`, `--tls-ca`** (`MODBUSCTL_TLS_CERT`, `MODBUSCTL_TLS_KEY`, `MODBUSCTL_TLS_CA`, the variables the client reads).
  - `--tls-cert` is the server certificate and turns TLS on. `--tls-key` defaults to the `--tls-cert` file, which may hold both.
  - `--tls-ca` is required with TLS: the CA certificates that sign the client certificates. Every client must present a certificate that bundle vouches for (mutual TLS, as the Modbus/TCP Security specification requires). A client without one, with one another CA signed, or speaking plain Modbus TCP gets no answer and is not counted as a request.
  - A missing or unreadable file, a key that does not match, `--tls-cert` without `--tls-ca`, or `--tls-key`/`--tls-ca` without `--tls-cert` is invalid input (exit 3), before the server binds.
  - The listener line reads `🚀 Serving Modbus TCP Security (tcp+tls, client certificates required) on … port N`. The port stays `--port` (default 502); Modbus/TCP Security uses 802 by convention.
  - The TLS handshake times out after 30 seconds (the go-modbus default).

### Tests

- A capture recorded over tcp+tls is served over tcp+tls by `server static` and `server replay` and read back with the modbusctl TLS client, through the library and through the CLI flags. The replay walks through the recorded values and ends on the last one; a read of it records the transport as `tls`.
- Both servers reject a client certificate of another CA, a server certificate used as a client certificate, a client that does not trust the server, and a plain Modbus TCP client, and serve none of their requests.
- Both servers fail as invalid input, before printing anything, without `--tls-ca`, without `--tls-cert`, with a certificate without its key, a mismatched key, or a missing, empty or certificate-less CA file. The server key may sit in the certificate file.

---

## v3.1.0

**Date:** 2026-10-15
**Previous release:** v3.0.0

### Summary

Minor release: modbusctl records and replays **every read function of the Modbus specification**: FC01–04, FC07, FC08 (sub-functions that return data), FC11, FC12, FC17, FC20, FC24 and FC43 Read Device Identification. **`client record`** takes them in its plan file, and **`server static`** and **`server replay`** answer them from the capture. Vendor and unknown function codes are not supported anywhere. The client speaks **Modbus/TCP Security**: every client command takes **`--tls-cert`**, **`--tls-key`** and **`--tls-ca`** for `tcp+tls://` URLs, which could not connect before. The **`mcap`** command group is renamed **`capture`**. Its `info`, `verify` and `convert` commands are removed in favour of [`mbcapctl`](https://github.com/otfabric/go-modbus-capture/blob/main/docs/cli.md) of go-modbus-capture. Adopts **go-modbus v1.7.1** and **go-modbus-capture v0.0.2**.

### Breaking changes (scripts)

- **`modbusctl mcap …` is now `modbusctl capture …`**, with `decode`, `extract`, `strings` and `frequencies`.
- **`mcap info`, `mcap verify` and `mcap convert` are removed.** Use `mbcapctl info`, `mbcapctl verify` and `mbcapctl convert IN OUT` (encoding from the file name; `--format wide-csv` for the wide view). Install with `go install github.com/otfabric/go-modbus-capture/cmd/mbcapctl@latest`. **`MODBUSCTL_FORMAT`** is no longer read.
- **JSON output:** `mcap_output_path` in the summaries of `client read`, `scan` and `record` is now **`capture_output_path`**.
- **Error codes:** `mcap_load_failed` is now **`capture_load_failed`** and `mcap_write_failed` is now **`capture_write_failed`**; exit codes are unchanged.
- **Docker:** the image volume is **`/captures`** (was `/mcap`); `docker-compose.yml` mounts it there and names `.mbcap` files.

### Added

- **Recording every spec read function:** a `--blocks-file` entry may name a `function`. FC01–04 blocks work as before (entries without `function` use `--function`). FC07, FC11, FC12 and FC17 take no fields. FC08 takes `sub_function` (0x00, 0x02 or 0x0B–0x12) and `data`. FC20 takes `file_records`, FC24 `fifo_address`, and FC43 `device_id_code` and `object_id`. Each request is retried like a read (busy device, dropped connection); a failed or retried attempt leaves no sample.
- **Serving FC08, FC17, FC20 and FC24:** both servers answer them with the data of the response recorded for exactly that request, as they already did for FC07, FC11 and FC12. FC08 sub-functions that change device state are answered with Illegal Function.
- **`--listen`** on both servers (`MODBUSCTL_LISTEN`): the address to listen on, for example `127.0.0.1`; the default is every interface, as before. The `🚀 Serving Modbus TCP …` line is now printed once the listener is bound.
- **Error code `capture_unsupported_function`** (exit 3): a capture file holding a sample outside the supported reads is refused when loaded.
- **Modbus/TCP Security on the client: `--tls-cert`, `--tls-key`, `--tls-ca`** on every client command (`MODBUSCTL_TLS_CERT`, `MODBUSCTL_TLS_KEY`, `MODBUSCTL_TLS_CA`), used with `tcp+tls://` URLs and ignored for other URLs. `tcp+tls://` URLs were accepted before but could not connect, because go-modbus requires a client certificate and modbusctl had no way to give one.
  - `--tls-cert` is required. `--tls-key` defaults to the `--tls-cert` file, which may hold both. `--tls-ca` defaults to the system roots.
  - A missing or unreadable file is invalid input (exit 3) and nothing is sent. A device that the CA does not vouch for, or that rejects the client certificate, is a transport error (exit 4).
  - Captures made over TLS record the target's transport as `tls`. The certificate files are not stored in them.

### Changed

- **Unsupported functions:** a plan entry with any other function code, or an FC08 sub-function that changes device state, is refused before anything is sent. The recorder never writes such a sample. Both servers answer such requests, and the writes FC21, FC22 and FC23, with Illegal Function.
- **Shorter 2.x message:** a modbusctl 2.x capture file (`.mcap`) is still refused (`capture_legacy_format`), with a shorter message.
- **go-modbus:** Requires **`github.com/otfabric/go-modbus` v1.7.1** (was v1.6.1).
- **go-modbus-capture:** Requires **`github.com/otfabric/go-modbus-capture` v0.0.2** (was v0.0.1), whose readers and writers accept only the read functions of the specification.

### Fixed

- **`client record` examples** gave `--interval` and `--duration` in seconds; both are milliseconds.
- **Flaky server tests:** tests picked a free port, released it and then started a server listening on every interface. Under load another socket could take the port in between. The bind then failed, or (on macOS) succeeded next to another test's loopback listener, and the test talked to the wrong device. Test servers now listen on `127.0.0.1` with `--listen`, wait for the server to report its bound listener, and choose another port only when the bind fails with "address already in use".

### Not included

- **No TLS server name override.** go-modbus checks the device certificate against the host of the URL and has no setting for another name.
- **No TLS on `server static` and `server replay`.** They serve over plain Modbus TCP, including captures recorded over TLS.

### Tests

- Record → file → replay round trips for every supported function. The sources are a go-modbus v1.7.1 server and the in-memory test device over TCP, RTU over TCP and UDP. Each file is checked with `mbcapctl verify` and converted to JSON Lines and CSV with `mbcapctl convert`. The static and replay servers answer byte for byte. There are chaos variants (busy, dropped, closed, truncated and late answers) and parallel variants (several recordings and many clients at once).
- Modbus/TCP Security, end to end against a go-modbus `tcp+tls` server with a throwaway CA:
  - A plan of FC03, FC08, FC17, FC20 and FC24 is recorded over TLS and replayed with `server replay`; the capture is also served back over TLS and read with the TLS client.
  - Every client command runs over TLS.
  - The client is refused by an untrusted CA, by the system roots, by a certificate without the device's address, and by a server that does not trust the client certificate.
  - It fails as invalid input with a missing certificate, a mismatched key, or a missing or empty CA file, in every client path.

---

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

---

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

---

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

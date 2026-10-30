# Process exit codes

`modbusctl` uses stable exit codes for scripting. A fatal error is rendered once: as one `error:` line on stderr, or as a single JSON `error` object on stdout when JSON is active. JSON is active when the command line has `--format json` or `MODBUSCTL_FORMAT` says so (the flag wins), and that holds for usage errors too: an unknown flag or command, a wrong argument or a malformed environment variable is answered with the JSON object, although no command ran.

| Code | Meaning |
|------|---------|
| 0 | Success (`ExitOK`) |
| 2 | Usage (`KindUsage`, code `invalid_usage`): unknown command or flag, a flag value of the wrong type, a required flag that is missing, wrong arguments. Also `confirmation_required`: `client write` or a state-changing sub-function of `client diagnostic` without `--yes` |
| 3 | Invalid input (`KindInvalidInput`): a flag value, a configuration or an input file that is not valid |
| 4 | Transport / connection (`KindTransport`): TCP/UDP dial failure or a serial port that cannot be opened |
| 5 | Timeout (`KindTimeout`): a request or network operation timed out (`request_timeout`, `transport_timeout`) and that is the command's fatal error, e.g. a timed-out `client read` or `client write`. SunSpec and FC08 diagnostics steps report a timeout under their own codes and exit `6` |
| 6 | Protocol / Modbus exception (`KindProtocol`, `KindModbus`) |
| 7 | Partial success (`ExitPartial`): a **valid** result was written to stdout, but aggregate semantics include embedded failures (e.g. any failed unit on `identify` / `reportserverid`, a `fingerprint` unit with no usable result or an interrupted partial probe, failed scan requests on `client scan`, or any failed poll on `client poll`) |
| 10 | Output / internal / unknown (`KindOutput`, `KindInternal`, or unrecognized errors) |
| 130 | Interrupted (`KindInterrupted`, code `context_canceled`): Ctrl-C (SIGINT) or SIGTERM before a one-shot command had its result |

**Kind vs code:** `kind` drives the exit mapping; `code` in JSON is stable for machines. Do not rely on parsing human `message` text for logic.

**Organisation table:** these codes follow the OT Fabric CLI exit-code table (ADR-0006 of otfabric/engineering). modbusctl does not use `1`, `8` and `9`: they keep their organisation meaning and are not reused for anything else.

**Changed in v3.3.0:**

| Case | Old | New | JSON `kind` |
|---|---|---|---|
| Interrupted before a result (`context_canceled`) | 10 | 130 | `interrupted`, was `internal` |
| Rejected by the command line parser (`invalid_usage`) | 10 | 2 | `usage` (the JSON object is new as well: the error went to stderr as text) |
| Input file not usable (`capture_load_failed`, `input_file_open_failed`, `input_decode_failed`) | 10 | 3 | `invalid_input`, was `output` |

**Partial exit:** Only after a successful formatted write of a success payload. Top-level transport, validation, or write failures use fatal codes above, never `7`.

**Streaming commands:** `client poll` writes one record per poll and keeps going after a failed poll, which it reports as a record carrying an `error`. It exits `0` when every poll succeeded and `7` when at least one failed. Stopping it with Ctrl-C (SIGINT) or SIGTERM ends the stream normally and does not change the exit code. Failing to connect in the first place is fatal (`4`).

**Capture files:** a modbusctl 2.x capture file (`capture_legacy_format`), and a `--target`/`--function` selection that matches several or no targets and functions of a capture (`capture_selection_ambiguous`, `capture_selection_empty`), and a capture holding a sample of a vendor or unknown function code, outside the Modbus read functions modbusctl supports (`capture_unsupported_function`), exit `3`. A capture file that cannot be read, or is corrupt or truncated where a command needs it complete (`capture_load_failed`), exits `3`; `server static` and `server replay` serve a truncated capture as far as it goes. Failing to write a capture (`capture_write_failed`) exits `10`.

**Interruption with a result:** an interrupted `fingerprint` writes its result and exits `7`, and `client poll` ends normally (see below). `server static` and `server replay` serve until they are stopped: Ctrl-C ends them normally with `0`.

**Format variable:** `MODBUSCTL_FORMAT` is the name to use; `MODBUSCTL_OUTPUT_FORMAT` stays accepted. When both are set, `MODBUSCTL_FORMAT` wins.

**Commands that change a device:** `client write` and the `client diagnostic` sub-functions `restartcommunications`, `changeasciiinputdelimiter`, `forcelistenonlymode`, `clearcountersanddiagnosticreg` and `clearoverruncounterandflag` need `--yes`. Without it they exit `2` (`confirmation_required`) and have sent nothing, whatever else is wrong with the command line, except an invalid `--format` (`3`). `--yes` has no environment variable. A sub-function that only reads does not need it.

**Writes:** `client write` validates everything before sending; a rejected command exits `2` or `3` and has sent nothing. A Modbus exception returned by the device exits `6`, a timeout `5`. A write is never retried.

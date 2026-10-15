# Process exit codes

`modbusctl` uses stable exit codes for scripting. Fatal errors are rendered once (stderr text, or a single JSON `error` object on stdout when `--format json` is active on a migrated client command).

| Code | Meaning |
|------|---------|
| 0 | Success (`ExitOK`) |
| 2 | Usage / CLI misuse (`KindUsage`) — reserved |
| 3 | Invalid input / bad flags or config (`KindInvalidInput`) |
| 4 | Transport / connection (`KindTransport`): TCP/UDP dial failure or a serial port that cannot be opened |
| 5 | Timeout (`KindTimeout`) — reserved for typed timeouts |
| 6 | Protocol / Modbus exception (`KindProtocol`, `KindModbus`) |
| 7 | Partial success (`ExitPartial`): a **valid** result was written to stdout, but aggregate semantics include embedded failures (e.g. any failed unit on `identify` / `reportserverid`, a `fingerprint` unit with no usable result or an interrupted partial probe, failed scan requests on `client scan`, or any failed poll on `client poll`) |
| 10 | Output / internal / unknown (`KindOutput`, `KindInternal`, or unrecognized errors) |

**Kind vs code:** `kind` drives the exit mapping; `code` in JSON is stable for machines. Do not rely on parsing human `message` text for logic.

**Partial exit:** Only after a successful formatted write of a success payload. Top-level transport, validation, or write failures use fatal codes above, never `7`.

**Streaming commands:** `client poll` writes one record per poll and keeps going after a failed poll, which it reports as a record carrying an `error`. It exits `0` when every poll succeeded and `7` when at least one failed. Stopping it with Ctrl-C (SIGINT) or SIGTERM ends the stream normally and does not change the exit code. Failing to connect in the first place is fatal (`4`).

**Capture files:** an MCAP v1 file from modbusctl 2.x (`capture_legacy_format`), and a `--target`/`--function` selection that matches several or no targets and functions of a capture (`capture_selection_ambiguous`, `capture_selection_empty`), and a capture holding a sample of a vendor or unknown function code, outside the Modbus read functions modbusctl supports (`capture_unsupported_function`), exit `3`. A capture file that cannot be read, or is corrupt or truncated where a command needs it complete (`mcap_load_failed`), exits `10`; `server static` and `server replay` serve a truncated capture as far as it goes. Failing to write a capture (`mcap_write_failed`) exits `10`.

**Writes:** `client write` validates everything before sending; a rejected command exits `3` and has sent nothing. A Modbus exception returned by the device exits `6`, a timeout `5`. A write is never retried.

# Process exit codes

`modbusctl` uses stable exit codes for scripting. Fatal errors are rendered once (stderr text, or a single JSON `error` object on stdout when `--format json` is active on a migrated client command).

| Code | Meaning |
|------|---------|
| 0 | Success (`ExitOK`) |
| 2 | Usage / CLI misuse (`KindUsage`) — reserved |
| 3 | Invalid input / bad flags or config (`KindInvalidInput`) |
| 4 | Transport / connection (`KindTransport`) |
| 5 | Timeout (`KindTimeout`) — reserved for typed timeouts |
| 6 | Protocol / Modbus exception (`KindProtocol`, `KindModbus`) |
| 7 | Partial success (`ExitPartial`): a **valid** result was written to stdout, but aggregate semantics include embedded failures (e.g. any failed unit on `identify` / `reportserverid`, a `fingerprint` unit with no usable result or an interrupted partial probe, or failed scan requests on `client scan`) |
| 10 | Output / internal / unknown (`KindOutput`, `KindInternal`, or unrecognized errors) |

**Kind vs code:** `kind` drives the exit mapping; `code` in JSON is stable for machines. Do not rely on parsing human `message` text for logic.

**Partial exit:** Only after a successful formatted write of a success payload. Top-level transport, validation, or write failures use fatal codes above, never `7`.

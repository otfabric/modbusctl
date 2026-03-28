# modbusctl Releases

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

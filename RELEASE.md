# modbusctl Releases

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
- **Local builds:** `make build` / `make build-nocheck` write the binary to **`bin/modbusctl`** (not the repository root). Docker image build copies from that path.

### Release workflow

- Releases are driven by the reusable **`go-binary-release`** workflow (see `.github/workflows/build-release.yml`), with ldflags aligned to the `main.*` variables above.

### Upgrading

- **Prebuilt binaries:** Download the same platform archive as before; add Windows zips if you deploy on Windows.
- **From source:** Use `make build` and run `./bin/modbusctl`, or adjust any scripts that assumed `./modbusctl` in the repo root.
- **Go modules:** Run `go mod tidy` after pulling; direct importers of the old `otfabric/modbus` path should depend on **`otfabric/go-modbus`** instead.

---

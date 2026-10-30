# AGENTS.md — modbusctl

## What this is

Command-line toolkit for Modbus devices over TCP, UDP, TCP Security and serial lines (RTU, ASCII), built on [go-modbus](https://github.com/otfabric/go-modbus) and [go-modbus-capture](https://github.com/otfabric/go-modbus-capture): read, poll, write, identify, fingerprint, scan, decode, SunSpec, device discovery, recording to MBCAP files and replay as a simulated device. Pure Go, one static binary. Profile: **cli**.

## Organisation standards

This repository follows **otfabric/engineering policy 1.0**, profile **cli** (`.otfabric/policy.yaml`, mode `baseline`). Where this file and the organisation standards disagree, this file wins here — say so in your report.

## Where the truth lives

| Question | Source |
|---|---|
| Exit codes (script-facing API) | docs/exitcodes.md, internal/errs |
| JSON output contract | docs/json-output.md |
| Command and environment reference | README.md, internal/config/types.go |
| Scan algorithms | ALGOS.md |
| Modbus behaviour | go-modbus |
| Release history | RELEASE.md (`## vX.Y.Z` … `---`) |

## Hard rules

1. Exit codes and JSON output are a public contract: changing a code or a field is a breaking change with a RELEASE.md note.
2. `client write` validates everything before sending and is never retried (docs/exitcodes.md).
3. Never lower `COVERAGE_MIN`, add lint suppressions, or skip or delete tests to make CI pass.
4. go-modbus and go-modbus-capture are pinned to released tags; no `replace` directives.
5. Format with `make fmt`: the gofmt of the Go version in go.mod is what CI uses, and `make fmt-check` needs the local gofmt to agree too.

## Commands

| When you change… | Run |
|---|---|
| anything | `make check` (needs staticcheck and golangci-lint) |
| tests only | `make test` |
| coverage | `make coverage` (whole module, `COVERAGE_MIN`) |

The tests need no hardware: `internal/mbtest` provides the Modbus device.

## Definition of Done

`make check` green; tests for new commands, including error paths; exit codes, JSON output and the README's environment table updated; RELEASE.md section for user-visible changes.

## Git and releases

Branches and PRs. Do not push to `main`, tag or run the Release workflow unless the maintainer asks in the session.

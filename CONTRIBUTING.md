# Contributing to modbusctl

Thank you for your interest in contributing to `modbusctl`! This document covers the guidelines for contributing.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```sh
   git clone git@github.com:<your-username>/modbusctl.git
   cd modbusctl
   ```
3. Create a feature branch:
   ```sh
   git checkout -b feature/my-change
   ```

## Development Setup

### Prerequisites

- Go 1.25+ (see [go.mod](go.mod))
- [staticcheck](https://staticcheck.dev/) and [golangci-lint](https://golangci-lint.run/) for linting
- No hardware: the test suite brings its own Modbus device (see [Testing](#testing))

### Build & Test

```sh
# Run all checks (format, lint, vet, race tests, coverage gate)
make check

# Run tests with the race detector
make test

# Run tests with whole-module coverage (fails below 85%)
make coverage

# Build for the current platform (./bin/modbusctl); runs `make check` first
make build
```

Run `make` (or `make help`) for the full list of targets.

## Making Changes

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Use meaningful variable and function names
- Add comments only where the logic isn't self-evident; comments end with a period (enforced by the linter)

### Formatting

`make fmt` formats with the `gofmt` of the Go version in `go.mod`, which is the version CI uses, not necessarily the one you have installed. `make fmt-check` (part of `make check`) fails if either that `gofmt` or your local one would change a file: `gofmt` output differs slightly between Go releases, and a file the two disagree on fails CI. If they disagree, restructure the code (for example, move trailing comments onto their own lines) rather than picking a side.

### Commit Messages

Use clear, concise commit messages:

```
component: short description

Optional longer explanation of the change, why it was made,
and any relevant context.
```

Examples:
- `poll: reconnect after a dropped connection`
- `scan: fix boundary search at the top of the address space`
- `docs: document serial settings`

### Testing

- Add tests for new functionality in the appropriate `_test.go` file
- Use [`internal/mbtest`](internal/mbtest) for anything that talks to a device. It is an in-memory Modbus slave that can be served over TCP, UDP, RTU or ASCII framing over TCP, and RTU or ASCII on a pseudo-terminal that stands in for a serial port (Linux and macOS; those tests skip elsewhere). It can also inject exceptions, timeouts, dropped connections, late answers and malformed replies.
- Use [`internal/capfile/capfiletest`](internal/capfile/capfiletest) to build capture fixtures and read captures back as register-block records.
- Code that records or serves captures has, besides unit tests, an end-to-end pipeline test (`internal/modbus/e2e_capture_test.go`: scan → extract → record → static and replay, over every network transport), chaos tests (`chaos_capture_test.go`: a device that fails at random, abusive clients, damaged capture files) and parallel tests (`parallel_capture_test.go`). Extend them when you change these paths, and check that a planted bug makes them fail.
- Assert behaviour (output, exit code, device state), not just that the code ran
- Tests must not write files into the repository: pass `--output` (or the equivalent config field) under `t.TempDir()`
- Keep tests fast and race-clean; CI runs them with `-race` and a 120 s per-package timeout
- Whole-module statement coverage must stay at or above 85% (`make coverage`)
- If your change affects command output, verify against a real Modbus device when possible

### Documentation

When you change commands, flags, output or exit codes, update:

- The command's `Short` / `Long` / `Example` help text
- [README.md](README.md), including the environment variable table for new flags
- [docs/json-output.md](docs/json-output.md) and [docs/exitcodes.md](docs/exitcodes.md) — `--format json` and exit codes are a stable contract for scripts
- [RELEASE.md](RELEASE.md), calling out anything that breaks existing usage

## Submitting Changes

1. Push your branch to your fork:
   ```sh
   git push origin feature/my-change
   ```
2. Open a Pull Request against the `main` branch
3. Describe what your change does and why
4. Reference any related issues

For larger changes, please open an issue first to discuss the approach.

## Reporting Issues

- Use GitHub Issues to report bugs or suggest features
- Include the output of `modbusctl version` in bug reports
- Provide the exact command that triggered the issue, along with the error output; `--debug` adds the Modbus frames sent and received
- Mention the transport (TCP, UDP, RTU, ASCII), the serial settings where relevant, and the device type/vendor
- Report security vulnerabilities privately: see [SECURITY.md](SECURITY.md)

## Project Structure

```
cmd/                   Root command, version, completion
cmd/client/            Client commands (identify, read, decode, poll, write, scan, record, sunspec, …)
cmd/discover/          Subnet device discovery
cmd/capture/           Capture analysis commands (decode, extract, strings, frequencies)
cmd/server/            Static and replay servers
internal/config/       Flag and environment binding, Modbus URL parsing, serial settings
internal/modbus/       Collectors, poll and write, scan strategies, recording, capture servers
internal/capfile/      Capture recorder (go-modbus PDU tap), loader, register-block adapter, error mapping
internal/replay/       Replay core: sample-and-hold lookup per address, request and identity object; replay clock
internal/analysis/     Blocks export and profile decode of capture files
internal/plan/         Recording plans (the requests of client record)
internal/format/       Output formatting (text, JSON, table, streaming)
internal/types/        Output data models
internal/validate/     Input validation
internal/runner/       Command wiring, formatted and streaming runs, fatal rendering
internal/errs/         Typed errors, stable codes, exit-code mapping
internal/mbtest/       In-memory Modbus device for tests
```

## License

By contributing to `modbusctl`, you agree that your contributions will be licensed under the [MIT License](LICENSE).

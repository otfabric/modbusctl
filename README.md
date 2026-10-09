# modbusctl — Modbus Command-Line Tool for TCP, RTU & ASCII: Poll, Write, Scan, Identify, SunSpec, Record & Simulate

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI Status](https://github.com/otfabric/modbusctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/modbusctl/actions/workflows/ci.yml)
[![Code Coverage](https://codecov.io/gh/otfabric/modbusctl/graph/badge.svg)](https://codecov.io/gh/otfabric/modbusctl)
[![Latest Release](https://img.shields.io/github/v/release/otfabric/modbusctl?label=release)](https://github.com/otfabric/modbusctl/releases)

**modbusctl is a command-line toolkit for exploring, operating, testing and simulating Modbus devices over TCP, UDP and serial lines (RTU and ASCII).** Read, poll and write coils and registers, identify a device, fingerprint which function codes it supports, scan its register map to find readable blocks, decode registers into typed values, discover Modbus devices on a subnet, record sessions to a compact capture file (MCAP) and replay them as a simulated device.

It is built for commissioning and site surveys, troubleshooting, reverse-engineering undocumented register maps, SunSpec inverter and meter discovery, and building hardware-free test setups. Output can be plain text, a table or JSON with stable exit codes, so it works in scripts and CI. It is written in Go on top of [go-modbus](https://github.com/otfabric/go-modbus).

**Quick links:** [Quick start](#quick-start) · [Comparison](#modbusctl-vs-mbpoll-and-modpoll) · [Features](#features) · [Transports](#transports-and-serial-settings) · [Scan algorithms](#scan-algorithms-detailed) · [MCAP format](#mcap-format) · [FAQ](#faq)

> **Responsible use:** modbusctl sends real Modbus requests. Only use it on devices and networks you own or are authorized to test. Some devices react badly to scanning, so start with `--algo safe` and a `--delay`. `client write` changes coils and registers, and FC08 diagnostic sub-functions can change device state (for example restart communications, clear counters or force listen-only mode): know what an address controls before you write to it.

---

## Why modbusctl?

- **Understand an unknown device** — `identify` (FC43/FC17), `fingerprint` (supported read functions per unit) and `client sunspec` tell you what you are talking to.
- **Find the register map** — `scan` discovers readable address blocks with seven strategies, from the gentle `safe` to the adaptive `smart` and evidence-driven `deep`.
- **Typed decoding** — `decode` turns registers into integers, floats, strings, BCD, timestamps, IP and MAC addresses with explicit byte and word layouts.
- **Poll and write** — `poll` reads a block at an interval and streams every sample; `write` sets coils and holding registers, with the same codecs for typed values.
- **Every common transport** — Modbus TCP, Modbus UDP, serial RTU and ASCII (RS-232/RS-485), and RTU or ASCII framing through a serial-to-Ethernet bridge, all selected with `--url`.
- **Record, replay, simulate** — Capture sessions to MCAP, then serve them with `server static` or `server replay` to develop and test without hardware.
- **Find devices** — `discover` sweeps subnets for Modbus devices, with a safety cap on large ranges.
- **Script-friendly** — `--format text|json|table`, a single JSON error object on failure, stable exit codes and `MODBUSCTL_*` environment variables.
- **Offline analysis** — Convert, decode, extract, summarize and string-search recorded data with the `mcap` commands.
- **Pre-built binaries** — Linux, macOS and Windows (x86-64 and ARM) from [GitHub Releases](https://github.com/otfabric/modbusctl/releases), with SHA-256 checksums; no runtime to install.

---

## Quick start

Download a binary from [GitHub Releases](https://github.com/otfabric/modbusctl/releases) (see [Installation](#installation) for copy-paste commands for Linux, macOS and Windows, or build from source), then point it at a device:

```console
# What is this device? (FC43 Device Identification + FC17 Report Server ID)
modbusctl client identify --ip 192.168.1.10 --server-id

# Which read function codes does each unit support?
modbusctl client fingerprint --ip 192.168.1.10 --unit 1-10

# Which register blocks are readable? (adaptive scan, saved to MCAP)
modbusctl client scan --ip 192.168.1.10 --algo smart --start 0 --end 5000 --output scan.mcap

# Read and decode a 32-bit float, as JSON
modbusctl client decode --ip 192.168.1.10 --start 40001 --codec float32/layout:4321 --format json

# Poll four holding registers every second until Ctrl-C
modbusctl client poll --ip 192.168.1.10 --start 100 --count 4

# Write a holding register on a serial RTU device
modbusctl client write --url rtu:///dev/ttyUSB0 --baud 9600 --unit 3 --start 100 --values 1234

# Serve the scan results as a simulated Modbus TCP device on port 1502
modbusctl server static --input scan.mcap --port 1502
```

SunSpec device? Run `modbusctl client sunspec probe --url tcp://192.168.1.10:502 --unit 1`.

Addresses are **raw Modbus PDU addresses** (0–65535), not "4xxxx" register labels. See [Read registers](#read-registers-single-or-block).

---

## modbusctl vs mbpoll and modpoll

[mbpoll](https://github.com/epsilonrt/mbpoll) and [modpoll](https://pypi.org/project/modpoll/) are well-known Modbus command-line tools focused on polling and writing registers. modbusctl covers the same ground — polling and writing over TCP, UDP, RTU and ASCII — and adds device exploration, scanning and simulation. This table shows where each tool fits.

| Capability | modbusctl | mbpoll | modpoll |
|---|:---:|:---:|:---:|
| Modbus TCP master | ✅ | ✅ | ✅ |
| Modbus RTU (serial) | ✅ | ✅ | ✅ |
| Modbus ASCII and UDP | ✅ | — | ✅ |
| RTU / ASCII framing over TCP (serial bridges) | ✅ | — | ✅ |
| Read coils, discrete inputs, input and holding registers | ✅ | ✅ | ✅ |
| Write coils and holding registers | ✅ | ✅ | ✅ |
| Continuous polling at an interval | ✅ | ✅ | ✅ |
| Typed values (ints, floats, strings, word/byte order) | ✅ | ✅ | ✅ |
| BCD, timestamp, IP and MAC decoders | ✅ | — | — |
| **Register-map scanning** (find readable blocks) | ✅ | — | — |
| **Device identification** (FC43) and Report Server ID (FC17) | ✅ | partial (RTU only) | — |
| **Diagnostics** (FC08 sub-functions) | ✅ | — | — |
| **Supported-function fingerprinting** | ✅ | — | — |
| **SunSpec discovery** | ✅ | — | — |
| **Subnet device discovery** | ✅ | — | — |
| **Record and replay** sessions | ✅ | — | — |
| **Simulate a device** from recorded data | ✅ | — | — |
| Export data to CSV / JSON | ✅ | — | CSV |
| JSON output and stable exit codes for scripting | ✅ | — | — |
| MQTT publishing | — | — | ✅ |
| Docker image | — | — | ✅ |
| Shell completion | ✅ | — | — |
| Install | release binaries | apt, brew, source | pip, pipx, Docker |
| Language / runtime | Go | C++ (libmodbus) | Python 3.10+ |
| License | MIT | GPL-3.0 | MIT |

✅ = documented feature · — = not documented / not available. Based on each project's public documentation as of October 2026. If you spot an inaccuracy, please [open an issue](https://github.com/otfabric/modbusctl/issues) and we will correct it.

**Choose modbusctl if** you want one tool to poll and write over TCP, UDP, RTU or ASCII and also explore or map an unfamiliar device, inventory devices, work with SunSpec, capture traffic for analysis, simulate a device from a recording, or script Modbus checks with JSON output.

**Choose mbpoll if** you want a small, classic poll-and-write tool that is available from your package manager.

**Choose modpoll if** you want config-file-driven polling with MQTT publishing, or a ready-made Docker setup.

They combine well: use modbusctl to discover and document a device's register map, then feed it into the long-running collector that fits your deployment.

---

## Use cases

- **Commissioning and site surveys** — Identify devices, check supported functions, read decoded values and set a coil or setpoint in seconds, on Ethernet or an RS-485 bus.
- **Live monitoring while you work** — Poll a few registers at an interval and watch them change, or pipe JSON Lines into a script.
- **Reverse-engineering register maps** — Scan for readable blocks, then use `mcap strings` and `mcap frequencies` to locate device names and measurements.
- **Hardware-free testing** — Record a real device once, then replay it in CI or on a laptop with `server replay`.
- **Solar and energy equipment** — Detect SunSpec markers, walk model chains and print address maps for inverters and meters.
- **Network inventory** — Sweep subnets for Modbus TCP devices and export the results as JSON.
- **Automation and CI** — Use `--format json`, exit codes and environment variables in scripts and pipelines.

---

## Table of Contents

- [Why modbusctl?](#why-modbusctl)
- [Quick start](#quick-start)
- [modbusctl vs mbpoll and modpoll](#modbusctl-vs-mbpoll-and-modpoll)
- [Use cases](#use-cases)
- [Features](#features)
- [Installation](#installation)
- [MCAP format](#mcap-format)
- [Usage](#usage)
  - [Transports and serial settings](#transports-and-serial-settings)
  - [Poll registers continuously](#poll-registers-continuously)
  - [Write coils and registers](#write-coils-and-registers)
- [Environment variable configuration](#environment-variable-configuration)
- [FAQ](#faq)
- [Project structure](#project-structure)
- [Development](#development)
- [Version history](#version-history)
- [Contributing](#contributing)
- [License](#license)

---

## Features

The following features are available:

| **Feature**      | **Description**                                                                                                           |
|------------------|---------------------------------------------------------------------------------------------------------------------------|
| Scan             | Identify available Modbus register blocks with pluggable algorithms: **safe**, **smart**, **deep**, **stepped**, **linear**, **boundary**, **sunspec**. See [Scan algorithms](#scan-algorithms-detailed) and [ALGOS.md](ALGOS.md) for details. |
| Identify         | Read Device Identification (FC43) from a Modbus device.                                                                   |
| Fingerprint      | Probe supported read functions (FC08/FC43/FC03/FC04/FC01/FC02/FC11/FC18/FC20) per unit: functions the device answers or recognises, not ones it rejects as illegal.          |
| Diagnostic       | Send FC08 Diagnostics requests (loopback, restart communications, diagnostic counters, etc.).                             |
| ReportServerID   | Send FC17 Report Server ID and display server ID, run indicator, and additional device data.                              |
| Read             | Retrieve raw data from specified Modbus registers (arbitrary count, MCAP capture).                                        |
| Poll             | Read the same block of coils or registers at an interval and print every sample as it arrives (text, JSON Lines or table), optionally decoded with a codec. Nothing is written to disk. |
| Write            | Write coils (FC05/FC15) and holding registers (FC06/FC16). Integer values, or typed values (floats, 32/64-bit integers, strings, …) encoded with a **codec**. |
| Transports       | Modbus TCP, Modbus UDP, serial RTU and ASCII, and RTU or ASCII framing over TCP, selected with **`--url`**. See [Transports and serial settings](#transports-and-serial-settings). |
| Decode (registers) | Read holding/input registers (FC03/FC04) and decode them into typed values (integers, floats, strings, BCD, time, IP, MAC, …) using a **codec** such as `float32/layout:4321` or `ascii/registers:8`. |
| Codecs           | List available register codecs (id, family, value kind, register/byte width) with optional family / value-kind / register-width filters. |
| Record           | Record Modbus responses into structured MCAP files for replay or analysis.                                                |
| Static           | Serve previously scanned Modbus data to simulate device behavior (static hosting).                                        |
| Replay           | Serve previously recorded Modbus data to simulate device behavior (dynamically updated).                                  |
| Convert          | Convert MCAP data into human-readable formats (CSV, JSON).                                                                |
| Decode           | Decode MCAP data using specific device profiles.                                                                          |
| Extract          | Extract raw data from recorded MCAP files.                                                                                |
| Info             | Display detailed metadata and statistics about MCAP files, including channel information, record counts, and time ranges. |
| Strings          | Extract all printable string data from MCAP files for inspection, debugging, or searching for embedded text.              |
| Frequencies      | Extract potential frequency readings around 50Hz from MCAP files to help finding the correct data ranges                  |
| Discovery        | Discover Modbus devices within specified subnets.                                                                         |
| SunSpec          | Transport-level SunSpec discovery: detect marker/base, enumerate model headers, print address map, or combined probe (no semantic decoding). |
| Client `--format` | Many **`modbusctl client`** commands accept **`--format text\|json\|table`** (or **`MODBUSCTL_OUTPUT_FORMAT`**) for structured stdout; **`mcap convert`** uses a different **`--format`** (**csv** / **json**). |
| Completion       | Generate shell completion scripts for bash, zsh, fish, or PowerShell; enum flags (**`--format`**, **`--algo`**, **`--function`**, **`--codec`**, **`--regtype`**, **`--sub-function`**, etc.) support tab completion. |

The following tree view gives the overview of available commands.

```
modbusctl
├── client
│   ├── identify         FC43 Read Device Identification (+ optional FC17 Report Server ID)
│   ├── fingerprint      Probe supported read functions per unit
│   ├── diagnostic       FC08 Diagnostics (loopback, counters, restart, …)
│   ├── reportserverid   FC17 Report Server ID
│   ├── read             Read registers (FC01/FC02/FC03/FC04)
│   ├── decode           Read and decode registers into typed values via a codec (FC03/FC04)
│   ├── codecs           List available register codecs for decode, poll and write
│   ├── poll             Poll coils/registers at an interval and print every sample
│   ├── write            Write coils (FC05/FC15) or holding registers (FC06/FC16)
│   ├── scan             Scan for valid register blocks (algo safe|smart|deep|stepped|linear|boundary|sunspec)
│   ├── record           Record registers over time to MCAP
│   └── sunspec           SunSpec marker detection and model header discovery
│       ├── detect       Detect SunSpec marker and base address
│       ├── models       Enumerate SunSpec model headers
│       ├── map          Print SunSpec address map summary
│       └── probe        Combined fingerprint + SunSpec detection summary
├── server
│   ├── static           Serve scanned MCAP data (static)
│   └── replay           Replay recorded MCAP data (dynamic)
├── mcap
│   ├── convert          Convert MCAP → JSON / CSV
│   ├── decode           Decode MCAP with device profiles
│   ├── extract          Extract address blocks from MCAP
│   ├── info             MCAP metadata & statistics
│   ├── strings          Extract printable strings from MCAP
│   └── frequencies      Detect ~50 Hz frequency data in MCAP
├── discover             Discover Modbus devices on subnets
├── completion           Generate shell completion script (bash|zsh|fish|powershell)
└── version              Print version
```

## Installation

The easiest way to install modbusctl is to download a pre-built binary from the [GitHub Releases page](https://github.com/otfabric/modbusctl/releases). No Go toolchain is required.

| Platform | CPU | Download |
|---|---|---|
| Linux | x86-64 | `modbusctl-<version>-linux-amd64.tar.gz` |
| Linux | 64-bit ARM (Raspberry Pi 4/5 with a 64-bit OS, AWS Graviton) | `modbusctl-<version>-linux-arm64.tar.gz` |
| Linux | 32-bit ARM (Raspberry Pi with a 32-bit OS) | `modbusctl-<version>-linux-armv7.tar.gz` |
| macOS | Apple Silicon (M1 and later) | `modbusctl-<version>-darwin-arm64.tar.gz` |
| macOS | Intel | `modbusctl-<version>-darwin-amd64.tar.gz` |
| Windows | x86-64 | `modbusctl-<version>-windows-amd64.zip` |
| Windows | ARM64 | `modbusctl-<version>-windows-arm64.zip` |

Each release also contains `modbusctl-<version>-checksums.txt` (SHA-256) so you can verify your download. The commands below fetch the **latest** release, verify its checksum and install it. Afterwards, check that it works:

```console
modbusctl version
```

### Linux

```bash
# 1. Find the latest version and your CPU architecture
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/modbusctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in
  x86_64)         ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  armv7l)         ARCH=armv7 ;;
  *) echo "Unsupported architecture: $(uname -m)" ;;
esac

# 2. Download the archive and the checksum file
FILE="modbusctl-${VERSION}-linux-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/modbusctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}"
curl -fsSLO "${BASE}/modbusctl-${VERSION}-checksums.txt"

# 3. Verify the download (should print "OK")
grep "${FILE}" "modbusctl-${VERSION}-checksums.txt" | sha256sum -c -

# 4. Extract and install (the binary in the archive carries the version and platform in its name)
mkdir -p modbusctl-dl && tar -xzf "${FILE}" -C modbusctl-dl
sudo install -m 0755 "$(find modbusctl-dl -type f -name 'modbusctl*' | head -n1)" /usr/local/bin/modbusctl
```

No `sudo`? Install into your home directory instead (and make sure `~/.local/bin` is in your `PATH`):

```bash
mkdir -p ~/.local/bin
install -m 0755 "$(find modbusctl-dl -type f -name 'modbusctl*' | head -n1)" ~/.local/bin/modbusctl
```

### macOS

```bash
# 1. Find the latest version and your CPU architecture (arm64 = Apple Silicon, x86_64 = Intel)
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/modbusctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in
  arm64)   ARCH=arm64 ;;
  x86_64)  ARCH=amd64 ;;
esac

# 2. Download the archive and the checksum file
FILE="modbusctl-${VERSION}-darwin-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/modbusctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}"
curl -fsSLO "${BASE}/modbusctl-${VERSION}-checksums.txt"

# 3. Verify the download (should print "OK")
grep "${FILE}" "modbusctl-${VERSION}-checksums.txt" | shasum -a 256 -c -

# 4. Extract and install (the binary in the archive carries the version and platform in its name)
mkdir -p modbusctl-dl && tar -xzf "${FILE}" -C modbusctl-dl
sudo install -m 0755 "$(find modbusctl-dl -type f -name 'modbusctl*' | head -n1)" /usr/local/bin/modbusctl
```

If you downloaded the archive with a web browser instead of `curl` and macOS refuses to run the binary ("cannot be verified"), remove the quarantine flag: `xattr -d com.apple.quarantine /usr/local/bin/modbusctl`.

### Windows (PowerShell)

```powershell
# 1. Find the latest version and your CPU architecture
$version = (Invoke-RestMethod https://api.github.com/repos/otfabric/modbusctl/releases/latest).tag_name.TrimStart('v')
$arch    = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }

# 2. Download the archive and the checksum file
$file = "modbusctl-$version-windows-$arch.zip"
$base = "https://github.com/otfabric/modbusctl/releases/download/v$version"
Invoke-WebRequest "$base/$file" -OutFile "$env:TEMP\$file"
Invoke-WebRequest "$base/modbusctl-$version-checksums.txt" -OutFile "$env:TEMP\modbusctl-checksums.txt"

# 3. Verify the download (throws an error on mismatch)
$expected = ((Select-String -Path "$env:TEMP\modbusctl-checksums.txt" -Pattern $file).Line -split '\s+')[0]
$actual   = (Get-FileHash "$env:TEMP\$file" -Algorithm SHA256).Hash
if ($expected -ne $actual) { throw "Checksum mismatch for $file" }

# 4. Extract to a per-user folder and name the program modbusctl.exe
#    (the .exe in the archive carries the version and platform in its name)
$dir = "$env:LOCALAPPDATA\Programs\modbusctl"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Expand-Archive "$env:TEMP\$file" -DestinationPath "$env:TEMP\modbusctl-dl" -Force
$exe = Get-ChildItem "$env:TEMP\modbusctl-dl" -Recurse -Filter 'modbusctl*.exe' | Select-Object -First 1
Copy-Item $exe.FullName "$dir\modbusctl.exe" -Force

# 5. Add modbusctl to your user PATH (then open a new terminal)
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
}
```

If Windows SmartScreen warns about an unrecognised app, choose **More info → Run anyway**.

### Manual download

1. Open the [latest release](https://github.com/otfabric/modbusctl/releases/latest).
2. Download the archive for your platform from the table above.
3. Extract it, rename the binary to `modbusctl` (`modbusctl.exe` on Windows) and move it to a folder on your `PATH`.

### Update

Run the same commands again. They always fetch the latest release and overwrite the previous binary.

### Build from source

Requires Go 1.25 or later:

```console
git clone https://github.com/otfabric/modbusctl.git
cd modbusctl
make build
```

`make build` writes the binary to **`bin/modbusctl`** (see [Building](#building)).

---

## MCAP Format

The `MCAP` (Modbus Capture) format is a specialized binary format designed for efficiently recording and replaying Modbus TCP communications, inspired by the well-known `PCAP` (Packet Capture) format used by tools such as Wireshark.

### Why MCAP?

MCAP provides a structured, compact, and efficient way to store Modbus data for:
 - Recording Modbus interactions with timestamps, iteration counters, and metadata.
 - Replaying recorded data for testing or simulation of Modbus devices.
 - Analyzing and decoding Modbus registers for debugging and diagnostics.

### MCAP File Structure

An MCAP file contains two main components, each encoded for a specific purpose:

| Component |Description |
|-----------|------------|
| Header    | Includes metadata such as IP address, port, function codes, timestamps, and protocol-specific details. The header is **JSON encoded** for readability and easy inspection. |
| Records   | Sequential Modbus transaction records containing iteration counts, precise request and response timestamps, register addresses, counts, and raw data payloads. The records are **binary encoded** for performance and compactness, enabling efficient storage and fast replay. |

The following is an example of a JSON converted MCAP file:

```json
{
  "header": {
    "ip": "192.168.2.201",
    "port": 502,
    "unit": 21,
    "function": 3,
    "start_time": "2025-07-14T13:05:56.998694708Z"
  },
  "records": [
    {
      "iteration": 0,
      "request_timestamp": "2025-07-14T13:14:24.701149758Z",
      "response_timestamp": "2025-07-14T13:14:24.951906811Z",
      "start_address": 4096,
      "register_count": 62,
      "raw_data": "000001a7000000f3000000f4000000f5000001a4000001a8000001a8ffffffff0000013f000001ab000001cf00000047000001cdffffffa0ffffffd1ffffffffffffffffffffffffffffffff000001270000004e00000068000000720000001400000024fffffff6fffffffbfffffee9ffffffbeffffff9bffffff90"
    },
    {
      "iteration": 1,
      "request_timestamp": "2025-07-14T13:14:31.312332629Z",
      "response_timestamp": "2025-07-14T13:14:31.570347588Z",
      "start_address": 4158,
      "register_count": 67,
      "raw_data": "000003b40000a790ffffffffffffffff0000c36effffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    }
  ]
}
```

More examples and test files can be found in the [results](./results) folder for quick exploration and verification.

---

## Usage

### General CLI Usage

**Global flag:** `--debug` — print debug information (available for all commands; per-command debug behavior is documented where applicable).

**Client address:** All client commands accept either `--url` (e.g. `tcp://192.168.1.10:502` or `rtu:///dev/ttyUSB0`) or `--ip` and optionally `--port` (Modbus TCP); the two styles are **mutually exclusive** (you must set one and cannot set both). See [Transports and serial settings](#transports-and-serial-settings).

**Client stdout format (`--format`):** Commands **identify**, **read**, **decode**, **write**, **poll**, **scan**, **record**, **reportserverid**, **fingerprint**, **diagnostic**, and **client sunspec** (detect, models, map, probe) accept **`--format text`** (default), **`json`**, or **`table`**, or set **`MODBUSCTL_OUTPUT_FORMAT`**. This controls **only** the final human- or machine-readable **result on stdout**. It is **not** the same as **`modbusctl mcap convert --format`**, which chooses **csv** or **json** export for MCAP files (env **`MODBUSCTL_FORMAT`**).

**Scan and record:** Live progress (scan lines, per-block lines, worst-case hints, **`--debug`** traces, record iteration messages) goes to **stderr**; **stdout** is a **single** formatted result after the run (summary for scan/record). When using **`--format json`**, redirect or separate streams if scripts must not mix progress with JSON.

**Fatal errors:** With `--format json`, client commands write a **single JSON error object** to stdout on failure (fields `kind`, `code`, `message`) and use stable exit codes. With **text** or **table**, fatals use a single **`error:`** line on stderr and leave stdout empty. See [docs/exitcodes.md](docs/exitcodes.md) for the exit-code table. Per-unit failures in JSON success payloads use a structured **`error`** object (`kind`, `code`, `message`) on each affected row where applicable (**identify**, **reportserverid**, **fingerprint**).

```console
modbusctl [command] [flags]

modbusctl client identify
modbusctl client fingerprint
modbusctl client diagnostic
modbusctl client reportserverid
modbusctl client read
modbusctl client decode
modbusctl client codecs
modbusctl client poll
modbusctl client write
modbusctl client scan
modbusctl client record
modbusctl client sunspec detect
modbusctl client sunspec models
modbusctl client sunspec map
modbusctl client sunspec probe

modbusctl server static
modbusctl server replay

modbusctl mcap convert
modbusctl mcap decode
modbusctl mcap extract
modbusctl mcap info
modbusctl mcap strings
modbusctl mcap frequencies

modbusctl discover
modbusctl version
```

**Shell completion:** Enum-style flags support tab completion, including **`--format`** (client: text/json/table; **`mcap convert`**: csv/json), **`--algo`**, **`--function`**, **`--codec`**, **`--parity`**, **`--regtype`**, and **`--sub-function`**. To enable completion, generate and source the script (see [Cobra’s shell completion guide](https://github.com/spf13/cobra/blob/main/shell_completions.md)); for example, `modbusctl completion bash` (or `zsh` / `fish` / `powershell`).

### Transports and serial settings

Every `modbusctl client` command talks to the device named by `--url`. The scheme selects the transport:

| URL | Transport |
|---|---|
| `tcp://host:port` | Modbus TCP (same as `--ip host --port port`) |
| `udp://host:port` | Modbus TCP framing over UDP |
| `rtu://<device>` | Modbus RTU on a serial line (RS-232 / RS-485) |
| `ascii://<device>` | Modbus ASCII on a serial line |
| `rtuovertcp://host:port` | RTU framing over TCP (serial-to-Ethernet bridge) |
| `rtuoverudp://host:port` | RTU framing over UDP |
| `asciiovertcp://host:port` | ASCII framing over TCP (serial-to-Ethernet bridge) |

`<device>` is the serial port: `rtu:///dev/ttyUSB0` on Linux, `rtu:///dev/cu.usbserial-1420` on macOS, `rtu://COM3` on Windows.

The serial line is configured with four flags, available on every client command and ignored for network URLs:

| Flag | Values | Default |
|---|---|---|
| `--baud` | any rate the adapter supports | 19200 |
| `--data-bits` | 5, 6, 7, 8 | 8 for `rtu://`, 7 for `ascii://` |
| `--parity` | `none`, `even`, `odd` | `none` |
| `--stop-bits` | 1, 2 | 2 without parity, 1 with parity |

```console
# RTU at 9600 8E1, unit 3
modbusctl client read --url rtu:///dev/ttyUSB0 --baud 9600 --parity even --unit 3 --start 0 --count 4

# ASCII at 9600 7E1
modbusctl client identify --url ascii:///dev/ttyUSB0 --baud 9600 --parity even --unit 1

# Modbus UDP
modbusctl client decode --url udp://192.168.1.10:502 --start 100 --codec uint32/layout:4321

# An RTU device behind a serial-to-Ethernet bridge
modbusctl client scan --url rtuovertcp://192.168.1.50:4001 --unit 7 --start 0 --end 200 --output scan.mcap
```

Good to know:

- **Defaults differ between tools.** Many devices ship as 9600 8N1; the Modbus specification default is 19200 8E1. If a serial device does not answer, check `--baud`, `--parity` and `--stop-bits` against its manual first.
- **One request at a time on a serial line.** With `rtu://` and `ascii://`, `--parallel` is ignored and units are queried one after the other.
- **Unit IDs matter on a bus.** A unit that is not present does not answer, so each missing unit costs one `--timeout`. Use a short timeout (for example `--timeout 300`) when sweeping units with `--unit 1-32`.
- **MCAP captures** made over a serial line store the device path in the header's `ip` field and port 0.
- **`server` and `discover` are Modbus TCP only.**
- UDP transports are not part of the Modbus specification; vendors use either `udp://` (MBAP framing) or `rtuoverudp://`. Try both if unsure.

### Common Commands

#### Read Device Identification (FC43)
By default the command requests all categories (basic + regular + extended). You can restrict to one or more categories with optional flags. Use `--unit` to target a single unit, a range, a list, or all units (1–255). Use `--server-id` to additionally query FC17 Report Server ID.

**Unit selection (`--unit`):**

| Value       | Meaning                   | Example           |
|------------|----------------------------|-------------------|
| `N`        | Single unit ID             | `--unit 1`        |
| `N-M`      | Range (inclusive)          | `--unit 1-10`     |
| `N,M,P`    | Comma-separated list       | `--unit 1,5,25`   |
| `N-M,P,...`| Mix of range(s) and list   | `--unit 1-10,255` |
| `all`      | All units 1–255            | `--unit all`      |

When probing multiple units, use `--parallel N` (default 10) to run probes concurrently. Use **`--format json`** or **`--format table`** for machine-readable or tabular output.

```console
# All categories (default), single unit
modbusctl client identify --ip 192.168.1.10

# Also retrieve FC17 Report Server ID data
modbusctl client identify --ip 192.168.1.10 --server-id

# Specific category or combine --basic, --regular, --extended
modbusctl client identify --ip 192.168.1.10 --basic
modbusctl client identify --ip 192.168.1.10 --basic --regular

# Unit: single, range, list, or all (with optional --parallel for multiple units)
modbusctl client identify --ip 192.168.1.10 --unit 1
modbusctl client identify --ip 192.168.1.10 --unit 1-10
modbusctl client identify --ip 192.168.1.10 --unit 1,5,25
modbusctl client identify --ip 192.168.1.10 --unit all
modbusctl client identify --ip 192.168.1.10 --unit all --parallel 10
```

**Category flags (optional):**

| Flag          | Category  | Objects included |
|---------------|-----------|------------------|
| *(none)*      | All       | Basic + Regular + Extended (single request) |
| `--basic`     | Basic     | VendorName, ProductCode, MajorMinorRevision |
| `--regular`   | Regular   | Basic + VendorUrl, ProductName, ModelName, UserApplicationName |
| `--extended`  | Extended  | Regular + vendor-specific objects (0x80–0xFF) |
| `--server-id` | FC17      | Server ID, run indicator status, additional data |

Example result (object names are looked up when known; reserved/extended IDs are labelled):

```console
🔍 Connecting to 192.168.1.10:502...
✅ Device Identification (DevID Code: 1, Conformity Level: 0x01, More Follows: 0x00, Next Object ID: 0, Object Count: 6)
 - Object 0: HUAWEI [VendorName]
 - Object 1: Smart Logger [ProductCode]
 - Object 2: V200R002C20SPC121 [MajorMinorRevision]
 - Object 3: [VendorUrl]
  FC17 Report Server ID (byte count: 5): 01 FF 48 55 41
```

#### Fingerprint Device (fingerprint)
Probe each requested unit with one request per read-style function code (FC08, FC43, FC03, FC04, FC01, FC02, FC11, FC18, FC20), using go-modbus `SupportsFunction`. Output is the list of supported read functions per unit. Use `--interval` (ms) to add a delay between probes to avoid overloading the device.

A function counts as **supported** when the unit answers it normally, or with an exception that shows it recognised the function (for example *Illegal Data Address* for a register that does not exist). *Illegal Function*, a gateway exception for a unit that is not there, and no answer at all count as not supported.

```console
# Fingerprint unit 1 (default)
modbusctl client fingerprint --ip 192.168.1.10

# Add delay between probes (e.g. 100 ms)
modbusctl client fingerprint --ip 192.168.1.10 --unit 1 --interval 100

# Fingerprint a range or all units
modbusctl client fingerprint --ip 192.168.1.10 --unit 1-10
modbusctl client fingerprint --ip 192.168.1.10 --unit all
```

Example output:

```console
🔍 Fingerprinting device at 192.168.1.10:502 (supported read functions per unit)...

--- Unit ID 1 ---
✅ Unit 1: supported read functions:
  Read Holding Registers (0x03)
  Read Input Registers (0x04)
  Diagnostics (0x08)
  Report Server ID (0x11)
  Read Device Identification (0x2B)
```

#### Diagnostics (FC08)
Send an FC08 Diagnostics request. Use `--sub-function` with the sub-function **name** (lowercase, e.g. `returnquerydata`, `clearcountersanddiagnosticreg`). Default is **returnquerydata** (loopback). Tab completion is available for `--sub-function`.

<details>
<summary>FC08 sub-function names and codes</summary>

| CLI name (--sub-function)        | Code   |
|----------------------------------|--------|
| returnquerydata                  | 0x0000 |
| restartcommunications            | 0x0001 |
| returndiagnosticregister         | 0x0002 |
| changeasciiinputdelimiter        | 0x0003 |
| forcelistenonlymode              | 0x0004 |
| clearcountersanddiagnosticreg    | 0x000A |
| returnbusmessagecount            | 0x000B |
| returnbuscommunicationerrorcount | 0x000C |
| returnbusexceptionerrorcount     | 0x000D |
| returnservermessagecount         | 0x000E |
| returnservernoresponsecount      | 0x000F |
| returnservernakcount             | 0x0010 |
| returnserverbusycount            | 0x0011 |
| returnbuscharacteroverruncount   | 0x0012 |
| clearoverruncounterandflag       | 0x0014 |

</details>

```console
# Loopback test (default: returnquerydata, data 0000)
modbusctl client diagnostic --ip 192.168.1.10

# Loopback with custom data
modbusctl client diagnostic --ip 192.168.1.10 --data A537

# Return Bus Message Count
modbusctl client diagnostic --ip 192.168.1.10 --sub-function returnbusmessagecount

# Specific unit
modbusctl client diagnostic --ip 192.168.1.10 --unit 5 --sub-function returnquerydata
```

Example output:

```console
🔍 Sending FC08 Diagnostics to 192.168.1.10:502 (unit 1, sub-function returnquerydata / 0x0000)...
✅ Diagnostics response:
  Sub-function: 0x0000 (ReturnQueryData)
  Data:         00 00
```

#### Report Server ID (FC17)
Send an FC17 Report Server ID request. Returns the server ID byte, run indicator status (ON/OFF), and any additional device-specific data.

```console
# Query server ID from unit 1
modbusctl client reportserverid --ip 192.168.1.10

# Query a range of units
modbusctl client reportserverid --ip 192.168.1.10 --unit 1-10

# Query all units in parallel
modbusctl client reportserverid --ip 192.168.1.10 --unit all --parallel 10
```

Example output (default **`--format text`**):

```console
🔍 Sending FC17 Report Server ID to tcp://192.168.1.10:502...
✅ Report Server ID (unit 1):
  Data: 01 FF 48 55 41
  Run Indicator: ON
```

#### Read registers (single or block)

Use **`--format`** for stdout (after any MCAP write). **`--output`** still writes MCAP when set.

**Addresses:** `--start`, scan `--start`/`--end`, and record block JSON use **raw Modbus PDU addresses** (16-bit values as on the wire: 0–65535). They are **not** “4xxxx minus 40001” style logical labels — e.g. `--start 40001` is literal address **40001**, not “register 1”.

```console
modbusctl client read --ip 192.168.1.10 --start 30001 --count 2
modbusctl client read --ip 192.168.1.10 --start 40001 --count 10 --output data.mcap
modbusctl client read --ip 192.168.1.10 --start 40001 --count 10 --format json
```

> **Note:** `read` is the raw-capture command (arbitrary register count, MCAP output). Typed
> interpretation of register values now lives in `client decode`. The former `--ascii` and
> `--byteswap` flags have been removed; use the corresponding codecs instead
> (e.g. `--codec ascii/registers:N`, or a layout-aware numeric codec such as `uint32/layout:2143`).

#### Decode registers (typed values via codecs)

`client decode` reads holding (FC3) or input (FC4) registers and decodes them into typed
values using a [go-modbus codec](https://github.com/otfabric/go-modbus/tree/main/codec). The
codec determines how many registers each value spans. Omit `--count` to decode a single value;
set `--count` to a multiple of the codec register width to decode an array of values. Run
`modbusctl client codecs` to list every available codec ID.

```console
modbusctl client decode --ip 192.168.1.10 --start 40001 --codec float32/layout:4321
modbusctl client decode --ip 192.168.1.10 --function 4 --start 0 --codec ascii/registers:8
modbusctl client decode --ip 192.168.1.10 --start 100 --codec uint32/layout:4321 --count 8 --format json
modbusctl client decode --ip 192.168.1.10 --start 200 --codec ip_addr
```

#### List codecs

```console
modbusctl client codecs
modbusctl client codecs --family float
modbusctl client codecs --registers 2 --format table
modbusctl client codecs --value-kind string --format json
```

#### Poll registers continuously

`client poll` reads the same block every `--interval` milliseconds and prints each sample as soon as it is read. It does not write a file; use [`client record`](#record-registers-over-time-interval-duration-based) to capture to MCAP.

It runs until `--polls` samples were taken, or until you press Ctrl-C when `--polls` is 0 (the default). `--function` selects coils (1), discrete inputs (2), holding registers (3, default) or input registers (4). With `--codec`, registers are decoded into typed values exactly as `client decode` does.

```console
# Four holding registers, once a second, until Ctrl-C
modbusctl client poll --ip 192.168.1.10 --start 100 --count 4

# A big-endian float32 from input registers, every 500 ms, 20 times
modbusctl client poll --ip 192.168.1.10 --function 4 --start 0 --codec float32/layout:4321 --interval 500 --polls 20

# Eight coils as JSON Lines
modbusctl client poll --ip 192.168.1.10 --function 1 --start 0 --count 8 --format json

# A serial RTU device
modbusctl client poll --url rtu:///dev/ttyUSB0 --baud 9600 --unit 3 --start 0 --count 2
```

Samples go to **stdout**, one per poll; a one-line summary goes to **stderr** when polling ends:

```console
# --format text (default): timestamp, poll number, address=value
2026-10-09T12:00:00.004Z  #1  100=1234 101=0 102=0 103=0
2026-10-09T12:00:01.004Z  #2  100=1235 101=0 102=0 103=0

# --format json: one object per line
{"poll":1,"timestamp":"2026-10-09T12:00:00.004Z","unit_id":1,"function":3,"start_address":200,"count":2,"address_step":2,"codec":"float32/layout:4321","rtt_ms":0.4,"raw_data_hex":"41 AC 00 00","values":[21.5]}

# --format table: header once, then one row per poll
poll      timestamp                 rtt_ms    8         9         10        11        error
-         -                         -         -         -         -         -         -
1         2026-10-09T12:00:00.004Z  0.4       1         0         1         1

# stderr, at the end
2 poll(s) of tcp://192.168.1.10:502 in 1.004s: 2 succeeded, 0 failed
```

A poll that fails — timeout, Modbus exception, lost connection — is printed as a sample with an error (`error` object in JSON) and polling continues, reconnecting when needed. The exit code is **0** when every poll succeeded and **7** (partial) when at least one failed; stopping with Ctrl-C is not an error. Polls start on a fixed cadence: if a read takes longer than `--interval`, the next one starts immediately.

#### Write coils and registers

`client write` writes coils (FC05, FC15) or holding registers (FC06, FC16). `--values` takes one value or a comma-separated list, written to consecutive addresses starting at `--start`.

| `--function` | Writes | Values |
|---|---|---|
| *(omitted)* | holding registers: FC06 for one register, FC16 for several | integers, or typed values with `--codec` |
| `6` | one holding register | one integer |
| `16` | 1–123 holding registers | integers, or typed values with `--codec` |
| `5` | one coil | `1`/`0`, `true`/`false`, `on`/`off` |
| `15` | 1–1968 coils | list of coil values |

Register values are `0`–`65535`, `0x`-prefixed hex, or `-32768`–`-1` (written as two's complement). With `--codec`, each value is encoded into as many registers as the codec needs, so floats, 32/64-bit integers, strings, IP addresses and timestamps can be written directly; `modbusctl client codecs` lists the codec IDs.

```console
# One holding register (FC06)
modbusctl client write --ip 192.168.1.10 --start 100 --values 1234

# Three holding registers (FC16)
modbusctl client write --ip 192.168.1.10 --start 100 --values 10,0x00FF,-1

# Switch coil 8 on (FC05)
modbusctl client write --ip 192.168.1.10 --function 5 --start 8 --values on

# Four coils (FC15)
modbusctl client write --ip 192.168.1.10 --function 15 --start 8 --values 1,0,1,1

# A big-endian float32 setpoint: two registers, FC16
modbusctl client write --ip 192.168.1.10 --start 200 --codec float32/layout:4321 --values 21.5

# Modbus RTU on a serial line
modbusctl client write --url rtu:///dev/ttyUSB0 --baud 9600 --parity even --unit 3 --start 100 --values 1234
```

```console
Writing 3 register(s) starting at address 100 using function code 16
  [100] 10 (0x000A)
  [101] 255 (0x00FF)
  [102] 65535 (0xFFFF)
✅ Wrote 3 register(s) to unit 1
```

Safeguards:

- **`--start` is required.** There is no default address for a write.
- **Everything is validated before anything is sent** — values, codec, quantity limits and the address range. A rejected command (exit code 3) has sent nothing.
- **A write is sent exactly once.** It is never retried, because a retry after a lost response could apply the write twice. If a write times out, read the value back before sending it again.
- A Modbus exception from the device (for example *Illegal Data Address* for a read-only register) is reported as an error with exit code 6.

#### Scan registers (discover valid address blocks)

Scan discovers which register ranges are readable and writes successful reads to an MCAP file. Choose the algorithm with `--algo`:

| Algorithm | Description | Use case |
|-----------|-------------|----------|
| **safe**  | Conservative linear probing with descending block sizes (125, 64, 32, 16, 8, 4, 2, 1). Tries largest first; on failure tries smaller; on full failure advances by one address. Optional one-off left-boundary probe after success following failures. Fixed delay, no retry storms. | Fragile devices, first pass, production. |
| **smart** | Adaptive interval splitting with a **priority queue** (smaller intervals first; tie-break by start). Starts with 125-register chunks, splits failed intervals in half until count 1. Efficient on sparse maps, discovers boundaries sooner. | Unknown devices, best default. |
| **deep**  | Runs **smart** first, then **evidence-driven refinement**: only where a failed interval is adjacent to a successful one, probes ±8 registers around that edge with counts [1, 2, 4, 8]. Cap 500 refinement tasks. | Lab work, reverse engineering, difficult devices. |
| **stepped** | Quick check at step positions (e.g. 0, 1000, 2000). At each step probes 6 reads (pos−1, pos, pos+1 × count 1 or 2); on any hit expands with 125,64,32,16,8,4 **strictly within** `--start`/`--end`. Use `--step` (default 1000) and optional `--step-half-offset` for step/2 positions. | Fast discovery of hotspots; full range with few reads when no hits. |
| **linear**  | 125-aligned blocks: probe 125 at a time; on success extend forward, then binary-search for max tail. When a 125 succeeds after a previous 125 failed, binary-search backwards for the real start. Emits a single gap probe between blocks to detect small islands. | Longest continuous blocks with minimal probes; good when registers are mostly contiguous. |
| **boundary** | Given a known-good read (`--seed-start`, `--seed-count`), expands left/right with exponential then binary search; **clamps** to `--start`/`--end` (no skip). Invalid or out-of-range seed → no tasks. | One known-good range; find maximal readable interval around it. |
| **sunspec** | SunSpec protocol-aware: probes candidate base addresses for the SunS marker, then walks the model chain — reads each header (2 regs) followed by the full model body (in chunks of ≤125 regs). No `--start`/`--end` needed. Use `--sunspec-base` to skip detection; `--sunspec-bases` for custom candidates; `--sunspec-max-models` / `--sunspec-max-span` for limits. | SunSpec-compliant devices; full model capture to MCAP. |

**Flags:** `--ip` (required), `--start`, `--end`, `--function` (3=holding, 4=input), `--delay` (ms between requests), `--algo` (safe | smart | deep | stepped | linear | boundary | sunspec), `--step` (stepped algo only, default 1000), `--step-half-offset` (stepped: also probe at step/2), `--retry-timeout` (0 or 1: retry once on timeout/transport error), `--seed-start` and `--seed-count` (boundary algo: known-good read), `--sunspec-base` (sunspec: known base, skip detection), `--sunspec-bases` (sunspec: custom candidate bases), `--sunspec-max-models` (sunspec: max headers, 0=256), `--sunspec-max-span` (sunspec: max address span, 0=unlimited), `--output` (MCAP file), **`--format`** (stdout summary: **text** | **json** | **table**). Use global **`--debug`** to print each read range (start, count, end) on **stderr** before attempting. Default algo is **safe**.

```console
# Safe (default): conservative, low device load
modbusctl client scan --ip 192.168.1.10 --start 0 --end 1000 --output scan.mcap

# Smart: interval splitting, good for sparse maps
modbusctl client scan --ip 192.168.1.10 --algo smart --start 0 --end 5000 --output scan.mcap

# Deep: smart + boundary refinement for hard devices
modbusctl client scan --ip 192.168.1.10 --algo deep --function 3 --start 0 --end 2000 --output scan.mcap

# With delay between requests (e.g. 100 ms)
modbusctl client scan --ip 192.168.1.10 --start 1 --end 65535 --delay 100 --output scan-results.mcap

# Input registers (function code 4)
modbusctl client scan --ip 192.168.1.10 --function 4 --algo smart --output input_scan.mcap

# Stepped: quick check every 1000 registers (override with --step 100, 2000, 10000, etc.)
modbusctl client scan --ip 192.168.1.10 --algo stepped --step 1000 --output scan.mcap
modbusctl client scan --ip 192.168.1.10 --algo stepped --step 2000 --start 0 --end 20000 --output scan.mcap

# Linear: 125-aligned blocks, extend forward then tail; backward to find real start after late hit
modbusctl client scan --ip 192.168.1.10 --algo linear --start 0 --end 5000 --output scan.mcap

# Boundary: expand from a known-good read (e.g. from a prior scan or stepped hit)
modbusctl client scan --ip 192.168.1.10 --algo boundary --seed-start 100 --seed-count 10 --start 0 --end 500 --output scan.mcap

# With global --debug: print each read range, result outcome, and strategy internals (phase, state, decisions) for all algos
modbusctl client scan --ip 192.168.1.10 --algo safe --delay 10 --output scan.mcap --debug

# SunSpec: auto-detect base address and walk model chain
modbusctl client scan --ip 192.168.1.10 --algo sunspec --output sunspec.mcap

# SunSpec: known base address (skip detection)
modbusctl client scan --ip 192.168.1.10 --algo sunspec --sunspec-base 40000 --output sunspec.mcap

# SunSpec: custom candidate bases
modbusctl client scan --ip 192.168.1.10 --algo sunspec --sunspec-bases "0,40000,50000" --output sunspec.mcap
```

For every algorithm, a one-line **worst-case hint** and all **live progress** (scanning banner, each **Block:** line, **`--debug`** traces) go to **stderr**. **Stdout** prints **one** final summary after the run (default **text**, or **json** / **table** via **`--format`**). The summary includes algorithm, total requests, success/failed counts, **exception/timeout/transport error breakdown** (when non-zero), blocks and registers captured, average response time, duration, and output path.

```console
# stderr (illustrative)
Scanning registers from 0 to 1000 with function code 3 (algo: smart)
Block: Start: 0, End: 124, Count: 125
Block: Start: 250, End: 311, Count: 62

# stdout (default text)
Algo: smart
Requests: 418
Success: 91
Failed: 327
Blocks captured: 91
Registers captured: 1842
Avg response time: 12 ms
Duration: 2m14s
Output: scan.mcap
```
When non-zero, the summary also reports **Exception**, **Timeout**, and **Transport error** counts (from outcome classification) so you can distinguish Modbus exceptions from transient failures.

##### Scan algorithms (detailed)

| Algo     | When to use |
|----------|-------------|
| **safe** | Fragile device; small range; predictable, conservative scan. |
| **smart** | **Best default.** Unknown device; medium/large range; efficient discovery. |
| **deep** | After smart; you want precise interval edges (evidence-driven refinement). |
| **stepped** | Huge range; cheap triage; quick reconnaissance (not a final mapper). |
| **linear** | Long contiguous maps; PLC-style devices; fast when fragmentation is low. |
| **boundary** | You have one known-good (start, count); find maximal readable interval around it. |
| **sunspec** | SunSpec device; protocol-aware base detection + model chain walk to MCAP. |

Failures are classified (success, Modbus exception, timeout, transport error) so strategies can avoid retrying on strong exception evidence (e.g. Illegal Data Address). For a full specification of each algorithm (state, transitions, worst-case counts, and examples suitable for re-implementation), see **[ALGOS.md](ALGOS.md)**. For the optional improvement roadmap (priority queue, stepped offsets, coalesce-smart, etc.), see **[REFACTOR.md](REFACTOR.md)**.

<details>
<summary>Scan algorithm internals (full descriptions of each algorithm)</summary>

- **safe** — Linear scan from `--start` to `--end`. At each address the scanner tries the largest allowed block first (up to 125 registers), then smaller sizes in a fixed descending order (125, 64, 32, 16, 8, 4, 2, 1). On success it records the block and jumps past it; if there were failures at this address before success, it emits one **left-boundary probe** (start−1, 1) to help find the exact edge (one-off; does not alter scan position). On total failure it advances by one register. No retry storms. If `--start` > `--end`, the strategy is done immediately. Best when you want predictable load and full coverage.

- **smart** — Divide-and-conquer over the same range using a **priority queue** (smaller intervals first; tie-break by start address). It starts with aligned 125-register chunks. If a chunk read fails and the chunk is larger than one register, it splits that chunk in half and re-queues both halves; if it succeeds, the chunk is recorded and not split further. This discovers readable regions and boundaries efficiently without sliding one address at a time. Best default for unknown devices.

- **deep** — Two phases. Phase 1 runs the same interval discovery as **smart**. Phase 2 refines **only where there is evidence of a boundary**: a failed interval whose range intersects the edge (e.g. [edge−1, edge+1]). It then probes ±8 registers around those edges with counts [1, 2, 4, 8], capped at 500 tasks. Use for difficult devices or when you need precise interval edges.

- **stepped** — Optimized for a quick pass over a large range. Step positions are `start`, `start+step`, … up to `end` (`--step`, default 1000). With `--step-half-offset`, it also adds positions at `start+step/2`, `start+step+step/2`, etc. (deduplicated, sorted). At each step it runs 6 probes: read 1 and 2 registers at `pos−1`, `pos`, `pos+1` (probe addresses may extend ±1 at range edges for boundary detection). If any probe succeeds it “expands” from that address with 125, 64, 32, 16, 8, 4; **expansion reads are strictly within** `--start`/`--end`. If none of the 6 probes succeed it moves to the next step. Worst case (no hits): steps × 6; the tool prints this at the start.

- **linear** — Finds maximum continuous blocks using 125-register reads. It probes 125-aligned blocks; on success extends forward then binary-searches for the maximum tail; when a 125 succeeds after a previous 125 failed it binary-searches backwards for the real start. When there is a gap between the end of one block and the next 125-block, it emits one **(blockEnd, 1) gap probe** to detect small readable islands (result is recorded but does not change probe/forward/backward state). Worst case with no hits: one read per 125-block in range.

- **boundary** — Starts from a known-good read (`--seed-start`, `--seed-count`). Expands left and right with exponential sizes (1, 2, 4, 8, 16, 32, 64, 125), then binary-searches for exact boundaries. **Clamps** to `--start`/`--end` (never skips a size). If the seed does not overlap the configured range or is invalid, the strategy is done immediately and emits no tasks. Use when you have one known-good address and want the maximal readable interval around it.

- **sunspec** — SunSpec protocol-aware scan. **Phase 1** probes candidate base addresses (default: 0, 40000, 50000, …) for the SunS marker (`0x5375`, `0x6E53`). **Phase 2** walks the model chain: reads each header (2 regs for model ID + length), then reads the full model body in chunks of ≤125 registers, until the end model (`0xFFFF`, 0), a read failure, or a configured limit (`--sunspec-max-models`, `--sunspec-max-span`). When `--sunspec-base` is set, detection is skipped. Does not require `--start`/`--end`. Worst case: B base probes + M × (1 + ⌈length/125⌉) reads. Use for SunSpec-compliant inverters, meters, and energy devices.

</details>

#### Record registers over time (interval, duration based)

Progress messages (e.g. each iteration and recorded block) go to **stderr**; **stdout** is a **single** final summary (**`--format text|json|table`**).

```console
modbusctl client record --ip 192.168.1.10 --blocks-file register-ranges.json --interval 1000 --duration 60000 --output session.mcap
modbusctl client record --ip 192.168.1.10 --blocks-file register-ranges.json --interval 1000 --duration 60000 --output session.mcap --format json
```

#### Statically host data (no changes in data)

```console
modbusctl server static --input session.mcap --port 1502
```

#### Replay recorded data (looped)

```console
modbusctl server replay --input session.mcap --port 1502 --loops 0
modbusctl server replay --input session.mcap --loops=0 --interval=1000
```

#### Convert data to json or csv

```console
modbusctl mcap convert --input session.mcap --format json --output data.json
modbusctl mcap convert --input session.mcap --format csv --output data.csv
```

#### Decode data using device profiles (decoding+scaling)

```console
modbusctl mcap decode --input data.mcap --profile device_profile.json --output output.json
```

#### Extract data address blocks (continuous readable address spaces)

```console
modbusctl mcap extract --input input.mcap --output register-ranges.json
```

#### Information data (meta data, timing statistics)

```console
modbusctl mcap info --input data.mcap --output info.txt
```

#### Search for strings data (device identification)

Extracts printable ASCII strings from MCAP data. The command tries four common byte/register layouts (ABCD, BADC, CDAB, DCBA) automatically and tags each hit with the layout that produced it (e.g. `[ABCD] [100-103] 4 regs: TEST`). No byte-order flag is needed.

```console
modbusctl mcap strings --input file.mcap --output strings.txt
modbusctl mcap strings --input file.mcap
```

#### Search for frequency data (data discovery)

```console
modbusctl mcap frequencies --input file.mcap --output frequencies.txt
```

#### Discover devices (nmap alike probing)

Large subnets are capped by default (unique host count); use **`--force-large-scan`** (or **`MODBUSCTL_DISCOVER_FORCE_LARGE`**) when you intentionally want to probe beyond that safety limit.

```console
modbusctl discover --subnets 192.168.1.0/24 --output discovered.json
sudo modbusctl discover --subnets 192.168.1.0/24,192.168.2.0/24 --resolve-mac --interface eth1 --output discovered.json
```

#### SunSpec (client sunspec)

Transport-level SunSpec discovery: detect the SunSpec "SunS" marker, enumerate model headers, print the address map, or run a combined probe. Use `--url` (e.g. `tcp://192.168.1.10:502`) or `--ip`/`--port`; `--unit` (0–255, full Modbus range); `--regtype` **holding** or **input** (tab-completed). No semantic decoding of points—only marker and model ID/length.

| Command | Description |
|--------|-------------|
| **detect** | Is the unit SunSpec? Shows base address; use `--bases 0,40000,50000` to probe specific bases, `--verbose` for attempt log, `--format json` for JSON. |
| **models** | Enumerate the SunSpec model chain (start/end address, model ID, length, end model). Use `--base` to skip detection when base is known; `--max-models`, `--max-address-span` to limit reads; `--format json` for JSON. |
| **map** | Human-friendly address map (marker regs + model ranges). Options: `--show-header-regs`, `--show-next`, `--compact`, `--format json`. |
| **probe** | One-shot summary: Modbus FC03/FC04/FC43 support plus SunSpec detection (base, model count, end model). Complements `fingerprint` and `identify`. |

```console
# Detect SunSpec and show base address
modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec detect --ip 192.168.1.10 --unit 1 --regtype holding --bases 0,40000,50000 --verbose --format json

# List model headers (most-used)
modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec models --ip 192.168.1.10 --unit 1 --base 40000 --max-models 64 --format json

# Address map view
modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec map --ip 192.168.1.10 --unit 1 --show-header-regs --compact

# Combined fingerprint + SunSpec summary
modbusctl client sunspec probe --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec probe --ip 192.168.1.10 --unit 1 --format json
```

SunSpec subcommands use the same `--format` / `MODBUSCTL_OUTPUT_FORMAT` as other client commands (`text` default, `json`, `table`). A hidden parent flag `--json` is deprecated and maps to `--format json`.

#### Shell completion (install)

Generate and source the completion script for your shell so subcommands and flags (including enum values like `--regtype`, `--algo`, `--function`, `--format` on client and `mcap convert`) are completed:

```console
# Bash
source <(modbusctl completion bash)

# Zsh
source <(modbusctl completion zsh)

# Fish
modbusctl completion fish | source
```

## Environment Variable Configuration

Modbusctl supports environment variables for easy automation. The global flag `--debug` (print debug information) is available on all commands and has no environment variable; use it to enable per-command debug output (e.g. scan: read ranges; read/record: retry wait messages). The environment variables supported (depending on the command) are:

<details>
<summary>All <code>MODBUSCTL_*</code> environment variables</summary>

| Variable               | Type     | Description                                                 | Flag            | Default    |
|------------------------|----------|-------------------------------------------------------------|-----------------|------------|
| **MODBUSCTL_ADDRESS**     | uint16   | Start address for **`client read`**, **`decode`**, **`poll`** and **`write`** (`write` has no default) | `--start`       | 1          |
| **MODBUSCTL_ALGO**        | string   | Scan algorithm: safe, smart, deep, stepped, linear, boundary, or sunspec | `--algo`        | safe       |
| **MODBUSCTL_BAUD**        | uint     | Serial baud rate for `rtu://` and `ascii://` URLs (0 = 19200) | `--baud`        | 0          |
| **MODBUSCTL_CODEC**       | string   | Codec ID for **`client decode`**, **`poll`** and **`write`** (e.g. float32/layout:4321, ascii/registers:8, ip_addr) | `--codec`       |            |
| **MODBUSCTL_COUNT**       | uint16   | Number of coils/registers to read (with a codec: a multiple of the codec width; 0 = one value) | `--count`       | 1          |
| **MODBUSCTL_DATA_BITS**   | uint     | Serial data bits, 5–8 (0 = 8 for `rtu://`, 7 for `ascii://`) | `--data-bits`   | 0          |
| **MODBUSCTL_DELAY**       | uint16   | Delay in ms between client scan requests                 | `--delay`       | 0          |
| **MODBUSCTL_DURATION**    | uint32   | Total duration to record in ms                           | `--duration`    | 60000      |
| **MODBUSCTL_END**         | uint16   | End register address                                     | `--end`         | 65535      |
| **MODBUSCTL_FORMAT**      | string   | **`mcap convert`** output type: **csv** or **json** (not client stdout) | `--format`      |            |
| **MODBUSCTL_FUNCTION**    | uint8    | Function code (1=coil, 2=discrete, 3=holding, 4=input); **`client write`**: 5, 6, 15 or 16 (0 = FC06 for one register, otherwise FC16) | `--function`    | 3 (write: 0) |
| **MODBUSCTL_INPUT**       | string   | Input MCAP file (`mcap` subcommands; `server static` / `server replay`) | `--input`       |            |
| **MODBUSCTL_BLOCKS_FILE** | string   | JSON address blocks for **`client record`** only         | `--blocks-file` |            |
| **MODBUSCTL_INTERFACE**   | string   | Network interface to use for discovery                   | `--interface`   | eth0       |
| **MODBUSCTL_INTERVAL**    | uint32   | Interval in ms (poll/record/replay iterations or fingerprint probes) | `--interval`    | 0 (fingerprint), 1000 (poll), 5000 (record/replay) |
| **MODBUSCTL_IP**          | string   | Modbus TCP device IP address                             | `--ip`          |            |
| **MODBUSCTL_LOOPS**       | uint16   | Number of times to loop the replay                       | `--loops`       | 0          |
| **MODBUSCTL_OUTPUT**      | string   | Output MCAP file or directory                            | `--output`      |            |
| **MODBUSCTL_DATA**            | string   | Hex-encoded request data for FC08 Diagnostics        | `--data`        |            |
| **MODBUSCTL_PARALLEL**        | uint16   | Concurrent probes when using `identify --unit all` (1-64); ignored on serial lines | `--parallel`    | 10         |
| **MODBUSCTL_PARITY**      | string   | Serial parity: none, even or odd                         | `--parity`      | none       |
| **MODBUSCTL_POLLS**       | uint32   | **`client poll`**: number of polls before exiting (0 = until interrupted) | `--polls`       | 0          |
| **MODBUSCTL_PORT**        | uint16   | Modbus TCP port or server port                           | `--port`        | 502        |
| **MODBUSCTL_PROFILE**     | string   | Device profile to decode                                 | `--profile`     |            |
| **MODBUSCTL_RESOLVE_MAC** | bool     | Resolve MAC addresses of discovered devices              | `--resolve-mac` | false      |
| **MODBUSCTL_DISCOVER_FORCE_LARGE** | bool | Allow **discover** when unique host count exceeds the built-in safety cap | `--force-large-scan` | false |
| **MODBUSCTL_SUB_FUNCTION**| string   | FC08 Diagnostics sub-function name, lowercase (e.g. returnquerydata, clearcountersanddiagnosticreg) | `--sub-function`| returnquerydata |
| **MODBUSCTL_START**       | uint16   | Start register address (**`client scan`**)               | `--start`       | 1          |
| **MODBUSCTL_STOP_BITS**   | uint     | Serial stop bits, 1 or 2 (0 = 2 without parity, 1 with parity) | `--stop-bits`   | 0          |
| **MODBUSCTL_STEP**        | uint16   | Stepped algo: step size (e.g. 100, 1000, 2000)           | `--step`        | 1000       |
| **MODBUSCTL_STEP_HALF_OFFSET** | bool | Stepped algo: also probe at step/2 positions            | `--step-half-offset` | false |
| **MODBUSCTL_RETRY_TIMEOUT**    | uint8  | Retry once on timeout/transport (0=no, 1=yes)           | `--retry-timeout`   | 0    |
| **MODBUSCTL_SEED_START**  | uint16   | Boundary algo: seed start address (known-good read)       | `--seed-start`  | 0          |
| **MODBUSCTL_SEED_COUNT**  | uint16   | Boundary algo: seed register count (1–125)                | `--seed-count`  | 0          |
| **MODBUSCTL_SUNSPEC_BASE** | uint16  | Sunspec algo: known base address (skip detection)         | `--sunspec-base` | 0         |
| **MODBUSCTL_SUNSPEC_BASES** | string | Sunspec algo: comma-separated candidate base addresses    | `--sunspec-bases` |          |
| **MODBUSCTL_SUNSPEC_MAX_MODELS** | int | Sunspec algo: max model headers to read (0=256)         | `--sunspec-max-models` | 0   |
| **MODBUSCTL_SUNSPEC_MAX_SPAN** | uint16 | Sunspec algo: max address span from base (0=no limit)  | `--sunspec-max-span` | 0     |
| **MODBUSCTL_SUBNETS**     | string   | Subnets to scan for Modbus devices (comma-separated)     | `--subnets`     |            |
| **MODBUSCTL_TIMEOUT**     | uint16   | Request timeout in ms and dial cap; **0 = 10000** ms default | `--timeout`     | 0 (most client commands) |
| **MODBUSCTL_UNIT**        | string   | Unit ID (1-255); for multi-unit commands: single, range (1-10), list (1,5,25), mixed (1-10,255), or `all` | `--unit` | 1       |
| **MODBUSCTL_URL**         | string   | Modbus URL for client commands: `tcp://`, `udp://`, `rtu://`, `ascii://`, `rtuovertcp://`, `rtuoverudp://` or `asciiovertcp://` (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port | `--url`  |         |
| **MODBUSCTL_VALUES**      | string   | **`client write`**: comma-separated values to write       | `--values`      |            |
| **MODBUSCTL_REGTYPE**     | string   | Register type for sunspec: holding or input | `--regtype` | holding |
| **MODBUSCTL_SUNSPEC_BASES** | string | Comma-separated base addresses to probe (sunspec detect) | `--bases` | 0,40000,50000,… |
| **MODBUSCTL_SUNSPEC_BASE** | uint16  | Known SunSpec base address; skip detection when set (sunspec models/map) | `--base` | 0 |
| **MODBUSCTL_SUNSPEC_MAX_MODELS** | int | Maximum model headers to read; 0 = 256 (sunspec models) | `--max-models` | 0 |
| **MODBUSCTL_SUNSPEC_MAX_SPAN** | uint16 | Maximum address span from base; 0 = no limit (sunspec models) | `--max-address-span` | 0 |
| **MODBUSCTL_VERBOSE**     | bool     | Show probe attempts or extra detail (e.g. sunspec detect) | `--verbose` | false |
| **MODBUSCTL_OUTPUT_FORMAT** | string | Client stdout format: **text**, **json**, or **table** (identify, read, decode, write, poll, scan, record, reportserverid, fingerprint, diagnostic, sunspec, …). Empty/unset defaults to **text**. Not used by `mcap convert` (see **MODBUSCTL_FORMAT**). | `--format` | text |

</details>


Example usage:

```console
MODBUSCTL_IP=192.168.1.10 MODBUSCTL_ADDRESS=40001 MODBUSCTL_COUNT=10 modbusctl client read
MODBUSCTL_URL=rtu:///dev/ttyUSB0 MODBUSCTL_BAUD=9600 MODBUSCTL_PARITY=even MODBUSCTL_UNIT=3 modbusctl client poll --start 0 --count 4
MODBUSCTL_IP=192.168.1.10 MODBUSCTL_ALGO=smart MODBUSCTL_START=0 MODBUSCTL_END=1000 MODBUSCTL_OUTPUT=scan.mcap modbusctl client scan
```

---

## FAQ

### What is modbusctl?

modbusctl is a command-line tool for Modbus over TCP, UDP and serial lines (RTU and ASCII). It reads, polls and writes coils and registers, identifies devices, fingerprints supported function codes, scans register maps, decodes registers into typed values, discovers devices on a network, and records and replays sessions to simulate devices.

### How do I find which Modbus registers a device exposes?

Run a scan, for example `modbusctl client scan --ip 192.168.1.10 --algo smart --start 0 --end 5000 --output scan.mcap`. Use `--algo safe` with `--delay` for fragile devices and `--algo deep` for precise block edges. See [Scan registers](#scan-registers-discover-valid-address-blocks).

### How do I read a float or string from Modbus registers on the command line?

Use `client decode` with a codec, for example `modbusctl client decode --ip 192.168.1.10 --start 40001 --codec float32/layout:4321`. List all codecs with `modbusctl client codecs`. See [Decode registers](#decode-registers-typed-values-via-codecs).

### How do I simulate a Modbus device without hardware?

Record or scan a real device to an MCAP file, then serve it with `modbusctl server static --input session.mcap --port 1502` (fixed values) or `modbusctl server replay --input session.mcap` (values replayed over time).

### Does modbusctl support Modbus RTU (serial)?

Yes. Pass a serial URL to any client command, for example `modbusctl client read --url rtu:///dev/ttyUSB0 --baud 9600 --parity even --unit 3 --start 0 --count 4`. Modbus ASCII (`ascii://`), Modbus UDP (`udp://`) and RTU or ASCII through a serial-to-Ethernet bridge (`rtuovertcp://`, `asciiovertcp://`) work the same way. See [Transports and serial settings](#transports-and-serial-settings).

### Can modbusctl write registers or coils?

Yes. `modbusctl client write --ip 192.168.1.10 --start 100 --values 1234` writes a holding register; `--function 5` or `15` writes coils, and `--codec` writes typed values such as floats. See [Write coils and registers](#write-coils-and-registers).

### How do I poll Modbus registers continuously?

Use `modbusctl client poll --ip 192.168.1.10 --start 100 --count 4 --interval 1000`. It prints a sample per poll until you press Ctrl-C (or after `--polls` samples), with `--format json` for one JSON object per line. See [Poll registers continuously](#poll-registers-continuously).

### My serial device does not answer. What should I check?

The serial settings must match the device exactly: `--baud`, `--parity`, `--stop-bits` and `--data-bits`. Many devices use 9600 8N1 (`--baud 9600 --parity none --stop-bits 1`), while modbusctl defaults to 19200 with no parity and 2 stop bits. Also check the `--unit` ID, that A/B are not swapped on RS-485, and whether the device speaks RTU (`rtu://`) or ASCII (`ascii://`).

### What is MCAP?

MCAP (Modbus Capture) is a compact capture format, inspired by PCAP, that stores Modbus transactions with timestamps for replay and analysis. See [MCAP format](#mcap-format).

### How do I use modbusctl in scripts and CI?

Pass `--format json` (or set `MODBUSCTL_OUTPUT_FORMAT=json`). Progress goes to stderr, a single result goes to stdout, and failures return one JSON error object with a stable exit code. See [General CLI usage](#general-cli-usage) and [docs/exitcodes.md](docs/exitcodes.md).

### How is modbusctl different from mbpoll and modpoll?

mbpoll and modpoll are polling and writing tools. modbusctl polls and writes too, and adds register-map scanning, device identification, SunSpec discovery, subnet discovery and record/replay simulation. See the [comparison](#modbusctl-vs-mbpoll-and-modpoll).

---


## Project structure

<details>
<summary>Repository layout</summary>

```console
modbusctl/
├── cmd/
│   ├── client (Identify, Fingerprint, Diagnostic, ReportServerID, Read, Decode, Poll, Write, Record, Scan, SunSpec commands)
│   ├── discover (Device discovery)
│   ├── mcap (MCAP file operations: convert, decode, extract, frequencies, info, strings)
│   └── server (Modbus TCP data replay server)
├── internal/
│   ├── config (CLI & env: types, flags, completion, diagnostics, Modbus URL / SunSpec base parsing)
│   ├── cli (shared Cobra helpers: enum completion, stdout file, required flags, debug)
│   ├── format (client stdout **text/json/table** via `Write`, streaming output via `StreamWriter`; MCAP-derived **strings** / **frequencies** heuristics; table rendering)
│   ├── mcap (MCAP binary codec, load, CSV/JSON/blocks/info exports, profile decode)
│   ├── runner (client command wiring, formatted run loop, fatal JSON/text rendering)
│   ├── errs (typed CLI errors, stable codes, exit-kind mapping)
│   ├── mbtest (in-memory Modbus device for tests: TCP, UDP, RTU/ASCII over TCP and on a pseudo-terminal)
│   ├── modbus (Modbus client/server logic, collectors, poll and write, scan strategies)
│   ├── types (DTOs for JSON/text/table output and MCAP-related structs)
│   └── validate (Input validation and checks)
├── Makefile (Build & test commands)
└── go.mod (Dependencies)
```

</details>

## Development

A toplevel `Makefile` is provided for convenience. Run **`make`** (or **`make help`**) for the full list. Common targets:

| Target | Purpose |
|--------|---------|
| **`make test`** | Unit tests with race detector (`go test -race ./...`) |
| **`make coverage`** | Tests + **`coverage.out`** across the whole module; fails below **85%** total statement coverage (`COVERAGE_MIN`) |
| **`make compile`** | Full-module typecheck (`go build ./...`) |
| **`make lint`** | `compile` then **staticcheck** |
| **`make lint-ci`** | `compile`, **go vet**, **golangci-lint** (uses `.golangci.yml`) |
| **`make fmt`** / **`fmt-check`** | Format with the gofmt of the Go version in `go.mod` (the one CI uses); `fmt-check` fails if that gofmt or the locally installed one would change any file |
| **`make check`** | `fmt`, `fmt-check`, `lint`, `lint-ci`, `vet`, `test`, **`coverage`** |
| **`make build`** | Runs **`check`** then writes **`bin/modbusctl`** |
| **`make build-all`** / **`release-all`** | Cross-compile and package archives |

### Building

`make build` writes the binary to **`bin/modbusctl`** (not the repo root).

```console
make build
make build-all
```

### Running Tests

```console
make test
make coverage
```

Tests need no hardware: `internal/mbtest` provides an in-memory Modbus device that is served over TCP, UDP, RTU/ASCII framing over TCP, and RTU/ASCII on a pseudo-terminal that stands in for a serial port (Linux and macOS; those tests are skipped elsewhere).

## Version History

Release notes for each version are maintained in **[RELEASE.md](RELEASE.md)** (e.g. migration notes and breaking changes).


---

## Contributing

Bug reports, device-compatibility notes and pull requests are welcome. Please [open an issue](https://github.com/otfabric/modbusctl/issues) to discuss larger changes first, and run `make check` before submitting. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and guidelines.

Please report security vulnerabilities privately, as described in [SECURITY.md](SECURITY.md), not in a public issue.

If modbusctl is useful to you, a star on GitHub helps other engineers find it.

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).

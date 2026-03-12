# modbusctl

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.txt)
[![Go Report Card](https://goreportcard.com/badge/github.com/otfabric/modbusctl)](https://goreportcard.com/report/github.com/otfabric/modbusctl)
[![CI](https://github.com/otfabric/modbusctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/modbusctl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/badge/release-v2.1.0-blue.svg)](https://github.com/otfabric/modbusctl/releases)

Modbusctl is a versatile command-line utility designed for efficient communication, scanning, recording, replaying, decoding, and analysis of Modbus TCP devices. It simplifies working with Modbus data through convenient CLI commands, environment variable integration, and flexible output formats (MCAP, JSON, CSV).

## 📖 Table of Contents

- [🚀 Features](#-features)
- [💾 MCAP Format](#-mcap-format)
  - [Why MCAP?](#why-mcap)
  - [MCAP File Structure](#mcap-file-structure)
- [🔧 Installation](#-installation)
- [📦 GitHub Releases](#-github-releases)
- [🖥️ Usage](#️-usage)
  - [General CLI Usage](#general-cli-usage)
  - [Common Commands](#common-commands)
- [🌍 Environment Variable Configuration](#-environment-variable-configuration)
- [📂 Project Structure](#-project-structure)
- [⚙️ Development](#️-development)
  - [Building](#building)
  - [Running Tests](#running-tests)
- [🗒️ Version History](#️-version-history)

## 🚀 Features

The following features are available:

| **Feature**      | **Description**                                                                                                           |
|------------------|---------------------------------------------------------------------------------------------------------------------------|
| Scan             | Identify available Modbus register blocks with pluggable algorithms: **safe**, **smart**, **deep**, **stepped**, **linear**, **boundary**. See [Scan algorithms](#scan-algorithms-detailed) and [ALGOS.md](ALGOS.md) for details. |
| Identify         | Read Device Identification (FC43) from a Modbus device.                                                                   |
| Fingerprint      | Probe supported read functions (FC08/FC43/FC03/FC04/FC01/FC02/FC11/FC18/FC20) per unit via HasUnitReadFunction.          |
| Diagnostic       | Send FC08 Diagnostics requests (loopback, restart communications, diagnostic counters, etc.).                             |
| ReportServerID   | Send FC17 Report Server ID and display server ID, run indicator, and additional device data.                              |
| Read             | Retrieve data from specified Modbus registers.                                                                            |
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
| Completion       | Generate shell completion scripts for bash, zsh, fish, or PowerShell.                                                    |

The following tree view gives the overiew of available commands.

```
modbusctl
├── client
│   ├── identify         FC43 Read Device Identification (+ optional FC17 Report Server ID)
│   ├── fingerprint      Probe supported read functions per unit (HasUnitReadFunction)
│   ├── diagnostic       FC08 Diagnostics (loopback, counters, restart, …)
│   ├── reportserverid   FC17 Report Server ID
│   ├── read             Read registers (FC01/FC02/FC03/FC04)
│   ├── scan             Scan for valid register blocks (algo safe|smart|deep|stepped|linear|boundary)
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

## 💾 MCAP Format

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
      "reponse_timestamp": "2025-07-14T13:14:24.951906811Z",
      "start_address": 4096,
      "register_count": 62,
      "raw_data": "000001a7000000f3000000f4000000f5000001a4000001a8000001a8ffffffff0000013f000001ab000001cf00000047000001cdffffffa0ffffffd1ffffffffffffffffffffffffffffffff000001270000004e00000068000000720000001400000024fffffff6fffffffbfffffee9ffffffbeffffff9bffffff90"
    },
    {
      "iteration": 1,
      "request_timestamp": "2025-07-14T13:14:31.312332629Z",
      "reponse_timestamp": "2025-07-14T13:14:31.570347588Z",
      "start_address": 4158,
      "register_count": 67,
      "raw_data": "000003b40000a790ffffffffffffffff0000c36effffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    }
  ]
}
```

More examples and test files can be found in the [results](./results) folder for quick exploration and verification.


## 🔧 Installation

Build from source (requires Go):

```console
git clone git@github.com:otfabric/modbusctl.git
cd modbusctl
make build build-all
```

The binary will be generated as `modbusctl`.

## 📦 GitHub Releases

Pre-built binaries for various platforms are available for download on the [GitHub Releases page](https://github.com/otfabric/modbusctl/releases). You can download the latest stable version without building from source.

## 🖥️ Usage

### General CLI Usage

**Global flag:** `--debug` — print debug information (available for all commands; per-command debug behavior is documented where applicable).

**Client address:** All client commands accept either `--url` (e.g. `tcp://192.168.1.10:502`) or `--ip` and optionally `--port`; the two styles are **mutually exclusive** (you must set one and cannot set both).

```console
modbusctl [command] [flags]

modbusctl client identify
modbusctl client fingerprint
modbusctl client diagnostic
modbusctl client reportserverid
modbusctl client read
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

**Shell completion:** Enum-style flags support tab completion (e.g. `--algo`, `--format`, `--function`, `--sub-function` for diagnostic). To enable completion in your shell, generate and source the completion script for your shell (see [Cobra’s shell completion guide](https://github.com/spf13/cobra/blob/main/shell_completions.md)); for example, `modbusctl completion bash` (or `zsh`/`fish`/`powershell`).

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

When probing multiple units, use `--parallel N` (default 10) to run probes concurrently.

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
Probe each requested unit with the library’s single-probe API `HasUnitReadFunction(ctx, unitId, fc)` for the supported read-style function codes (FC08, FC43, FC03, FC04, FC01, FC02, FC11, FC18, FC20). Output is the list of supported read functions per unit. Use `--interval` (ms) to add a delay between probes to avoid overloading the device.

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

Example output:

```console
🔍 Sending FC17 Report Server ID to 192.168.1.10:502...
✅ Report Server ID (unit 1, byte count: 5):
  Data: 01 FF 48 55 41
  Server ID: 0x01 (1)
  Run Indicator: 0xFF (ON)
  Additional Data: 48 55 41
```

#### Read registers (single or block)

```console
modbusctl client read --ip 192.168.1.10 --start 30001 --count 2
modbusctl client read --ip 192.168.1.10 --start 40001 --count 10 --output data.mcap
modbusctl client read --ip 192.168.1.10 --start 40001 --count 10 --ascii
modbusctl client read --ip 192.168.1.10 --start 40001 --count 10 --ascii --swap-bytes
```

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

**Flags:** `--ip` (required), `--start`, `--end`, `--function` (3=holding, 4=input), `--delay` (ms between requests), `--algo` (safe | smart | deep | stepped | linear | boundary), `--step` (stepped algo only, default 1000), `--step-half-offset` (stepped: also probe at step/2), `--retry-timeout` (0 or 1: retry once on timeout/transport error), `--seed-start` and `--seed-count` (boundary algo: known-good read), `--output` (MCAP file). Use global `--debug` to print each read range (start, count, end) before attempting. Default algo is **safe**.

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
```

For every algorithm, a one-line **worst-case hint** is printed at the start (how many reads if there were no hits at all), using your `--start` and `--end` range. At the end of a scan, summary statistics are printed: algorithm, total requests, success/failed counts, **exception/timeout/transport error breakdown** (when non-zero), blocks and registers captured, average response time, duration, and output path. Example:

```console
Scanning registers from 0 to 1000 with function code 3 (algo: smart)
Block: Start: 0, End: 124, Count: 125
Block: Start: 250, End: 311, Count: 62
...

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

Failures are classified (success, Modbus exception, timeout, transport error) so strategies can avoid retrying on strong exception evidence (e.g. Illegal Data Address). For a full specification of each algorithm (state, transitions, worst-case counts, and examples suitable for re-implementation), see **[ALGOS.md](ALGOS.md)**. For the optional improvement roadmap (priority queue, stepped offsets, coalesce-smart, etc.), see **[REFACTOR.md](REFACTOR.md)**.

- **safe** — Linear scan from `--start` to `--end`. At each address the scanner tries the largest allowed block first (up to 125 registers), then smaller sizes in a fixed descending order (125, 64, 32, 16, 8, 4, 2, 1). On success it records the block and jumps past it; if there were failures at this address before success, it emits one **left-boundary probe** (start−1, 1) to help find the exact edge (one-off; does not alter scan position). On total failure it advances by one register. No retry storms. If `--start` > `--end`, the strategy is done immediately. Best when you want predictable load and full coverage.

- **smart** — Divide-and-conquer over the same range using a **priority queue** (smaller intervals first; tie-break by start address). It starts with aligned 125-register chunks. If a chunk read fails and the chunk is larger than one register, it splits that chunk in half and re-queues both halves; if it succeeds, the chunk is recorded and not split further. This discovers readable regions and boundaries efficiently without sliding one address at a time. Best default for unknown devices.

- **deep** — Two phases. Phase 1 runs the same interval discovery as **smart**. Phase 2 refines **only where there is evidence of a boundary**: a failed interval whose range intersects the edge (e.g. [edge−1, edge+1]). It then probes ±8 registers around those edges with counts [1, 2, 4, 8], capped at 500 tasks. Use for difficult devices or when you need precise interval edges.

- **stepped** — Optimized for a quick pass over a large range. Step positions are `start`, `start+step`, … up to `end` (`--step`, default 1000). With `--step-half-offset`, it also adds positions at `start+step/2`, `start+step+step/2`, etc. (deduplicated, sorted). At each step it runs 6 probes: read 1 and 2 registers at `pos−1`, `pos`, `pos+1` (probe addresses may extend ±1 at range edges for boundary detection). If any probe succeeds it “expands” from that address with 125, 64, 32, 16, 8, 4; **expansion reads are strictly within** `--start`/`--end`. If none of the 6 probes succeed it moves to the next step. Worst case (no hits): steps × 6; the tool prints this at the start.

- **linear** — Finds maximum continuous blocks using 125-register reads. It probes 125-aligned blocks; on success extends forward then binary-searches for the maximum tail; when a 125 succeeds after a previous 125 failed it binary-searches backwards for the real start. When there is a gap between the end of one block and the next 125-block, it emits one **(blockEnd, 1) gap probe** to detect small readable islands (result is recorded but does not change probe/forward/backward state). Worst case with no hits: one read per 125-block in range.

- **boundary** — Starts from a known-good read (`--seed-start`, `--seed-count`). Expands left and right with exponential sizes (1, 2, 4, 8, 16, 32, 64, 125), then binary-searches for exact boundaries. **Clamps** to `--start`/`--end` (never skips a size). If the seed does not overlap the configured range or is invalid, the strategy is done immediately and emits no tasks. Use when you have one known-good address and want the maximal readable interval around it.

#### Record registers over time (interval, duration based)

```console
modbusctl client record --ip 192.168.1.10 --input register-ranges.json --interval 1000 --duration 60000 --output session.mcap
```

####  Statically host data (no changes in data)

```console
modbusctl server static --input session.mcap --port 1502
```

####  Replay recorded data (looped)

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

```console
modbusctl discover --subnets 192.168.1.0/24 --output discovered.json
sudo modbusctl discover --subnets 192.168.1.0/24,192.168.2.0/24 --resolve-mac --interface eth1 --output discovered.json
```

#### SunSpec (client sunspec)

Transport-level SunSpec discovery: detect the SunSpec "SunS" marker, enumerate model headers, print the address map, or run a combined probe. Use `--url` (e.g. `tcp://192.168.1.10:502`) or `--ip`/`--port`; `--unit` (1–247); `--regtype` **holding** or **input** (tab-completed). No semantic decoding of points—only marker and model ID/length.

| Command | Description |
|--------|-------------|
| **detect** | Is the unit SunSpec? Shows base address; use `--bases 0,40000,50000` to probe specific bases, `--verbose` for attempt log, `--json` for JSON. |
| **models** | Enumerate the SunSpec model chain (start/end address, model ID, length, end model). Use `--base` to skip detection when base is known; `--max-models`, `--max-address-span` to limit reads; `--json` for JSON. |
| **map** | Human-friendly address map (marker regs + model ranges). Options: `--show-header-regs`, `--show-next`, `--compact`, `--json`. |
| **probe** | One-shot summary: Modbus FC03/FC04/FC43 support plus SunSpec detection (base, model count, end model). Complements `fingerprint` and `identify`. |

```console
# Detect SunSpec and show base address
modbusctl client sunspec detect --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec detect --ip 192.168.1.10 --unit 1 --regtype holding --bases 0,40000,50000 --verbose --json

# List model headers (most-used)
modbusctl client sunspec models --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec models --ip 192.168.1.10 --unit 1 --base 40000 --max-models 64 --json

# Address map view
modbusctl client sunspec map --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec map --ip 192.168.1.10 --unit 1 --show-header-regs --compact

# Combined fingerprint + SunSpec summary
modbusctl client sunspec probe --url tcp://192.168.1.10:502 --unit 1
modbusctl client sunspec probe --ip 192.168.1.10 --unit 1 --json
```

#### Shell completion (install)

Generate and source the completion script for your shell so subcommands and flags (including enum values like `--regtype`, `--algo`, `--function`) are completed:

```console
# Bash
source <(modbusctl completion bash)

# Zsh
source <(modbusctl completion zsh)

# Fish
modbusctl completion fish | source
```

## 🌍 Environment Variable Configuration

Modbusctl supports environment variables for easy automation. The global flag `--debug` (print debug information) is available on all commands and has no environment variable; use it to enable per-command debug output (e.g. scan: read ranges; read/record: retry wait messages). The environment variables supported (depending on the command) are:

| Variable               | Type     | Description                                                 | Flag            | Default    |
|------------------------|----------|-------------------------------------------------------------|-----------------|------------|
| **MODBUSCTL_ALGO**        | string   | Scan algorithm: safe, smart, deep, stepped, linear, or boundary | `--algo`        | safe       |
| **MODBUSCTL_COUNT**       | uint16   | Number of registers to read                              | `--count`       | 1          |
| **MODBUSCTL_DELAY**       | uint16   | Delay in ms between client scan requests                 | `--delay`       | 0          |
| **MODBUSCTL_DURATION**    | uint32   | Total duration to record in ms                           | `--duration`    | 60000      |
| **MODBUSCTL_END**         | uint16   | End register address                                     | `--end`         | 65535      |
| **MODBUSCTL_FORMAT**      | string   | Output format type (e.g., CSV, JSON)                     | `--format`      |            |
| **MODBUSCTL_FUNCTION**    | uint8    | Function code (1=coil, 2=discrete, 3=holding, 4=input)   | `--function`    | 3          |
| **MODBUSCTL_INPUT**       | string   | Input MCAP file or file to replay                        | `--input`       |            |
| **MODBUSCTL_INTERFACE**   | string   | Network interface to use for discovery                   | `--interface`   | eth0       |
| **MODBUSCTL_INTERVAL**    | uint32   | Interval in ms (record/replay iterations or fingerprint probes) | `--interval`    | 0 (fingerprint), 5000 (record/replay) |
| **MODBUSCTL_IP**          | string   | Modbus TCP device IP address                             | `--ip`          |            |
| **MODBUSCTL_LOOPS**       | uint16   | Number of times to loop the replay                       | `--loops`       | 0          |
| **MODBUSCTL_OUTPUT**      | string   | Output MCAP file or directory                            | `--output`      |            |
| **MODBUSCTL_DATA**            | string   | Hex-encoded request data for FC08 Diagnostics        | `--data`        |            |
| **MODBUSCTL_PARALLEL**        | uint16   | Concurrent probes when using `identify --unit all` (1-64) | `--parallel`    | 10         |
| **MODBUSCTL_PORT**        | uint16   | Modbus TCP port or server port                           | `--port`        | 502        |
| **MODBUSCTL_PROFILE**     | string   | Device profile to decode                                 | `--profile`     |            |
| **MODBUSCTL_RESOLVE_MAC** | bool     | Resolve MAC addresses of discovered devices              | `--resolve-mac` | false      |
| **MODBUSCTL_SUB_FUNCTION**| string   | FC08 Diagnostics sub-function name, lowercase (e.g. returnquerydata, clearcountersanddiagnosticreg) | `--sub-function`| returnquerydata |
| **MODBUSCTL_START**       | uint16   | Start register address                                   | `--start`       | 1          |
| **MODBUSCTL_STEP**        | uint16   | Stepped algo: step size (e.g. 100, 1000, 2000)           | `--step`        | 1000       |
| **MODBUSCTL_STEP_HALF_OFFSET** | bool | Stepped algo: also probe at step/2 positions            | `--step-half-offset` | false |
| **MODBUSCTL_RETRY_TIMEOUT**    | uint8  | Retry once on timeout/transport (0=no, 1=yes)           | `--retry-timeout`   | 0    |
| **MODBUSCTL_SEED_START**  | uint16   | Boundary algo: seed start address (known-good read)       | `--seed-start`  | 0          |
| **MODBUSCTL_SEED_COUNT**  | uint16   | Boundary algo: seed register count (1–125)                | `--seed-count`  | 0          |
| **MODBUSCTL_SUBNETS**     | string   | Subnets to scan for Modbus devices (comma-separated)     | `--subnets`     |            |
| **MODBUSCTL_TIMEOUT**     | uint16   | Timeout in milliseconds for the request(s)               | `--timeout`     | 2000       |
| **MODBUSCTL_UNIT**        | string   | Unit ID (1-255); for multi-unit commands: single, range (1-10), list (1,5,25), mixed (1-10,255), or `all` | `--unit` | 1       |
| **MODBUSCTL_URL**         | string   | Modbus URL for client commands (e.g. tcp://192.168.1.10:502); overrides --ip/--port when set (e.g. sunspec) | `--url`  |         |
| **MODBUSCTL_REGTYPE**     | string   | Register type for sunspec: holding or input | `--regtype` | holding |
| **MODBUSCTL_SUNSPEC_BASES** | string | Comma-separated base addresses to probe (sunspec detect) | `--bases` | 0,40000,50000,… |
| **MODBUSCTL_SUNSPEC_BASE** | uint16  | Known SunSpec base address; skip detection when set (sunspec models/map) | `--base` | 0 |
| **MODBUSCTL_SUNSPEC_MAX_MODELS** | int | Maximum model headers to read; 0 = 256 (sunspec models) | `--max-models` | 0 |
| **MODBUSCTL_SUNSPEC_MAX_SPAN** | uint16 | Maximum address span from base; 0 = no limit (sunspec models) | `--max-address-span` | 0 |
| **MODBUSCTL_VERBOSE**     | bool     | Show probe attempts or extra detail (e.g. sunspec detect) | `--verbose` | false |
| **MODBUSCTL_JSON**        | bool     | Output JSON instead of human-readable (e.g. sunspec commands) | `--json` | false |

Example usage:

```console
MODBUSCTL_IP=192.168.1.10 MODBUSCTL_START=40001 MODBUSCTL_COUNT=10 modbusctl client read
MODBUSCTL_IP=192.168.1.10 MODBUSCTL_ALGO=smart MODBUSCTL_START=0 MODBUSCTL_END=1000 MODBUSCTL_OUTPUT=scan.mcap modbusctl client scan
```

## 📂 Project Structure

```console
modbusctl/
├── cmd/
│   ├── client (Identify, Fingerprint, Diagnostic, ReportServerID, Read, Record, Scan, SunSpec commands)
│   ├── discover (Device discovery)
│   ├── mcap (MCAP file operations: convert, decode, extract, frequencies, info, strings)
│   └── server (Modbus TCP data replay server)
├── internal/
│   ├── config (CLI & ENV config management)
│   ├── format (MCAP, JSON, CSV format handling)
│   ├── modbus (Modbus TCP client/server logic)
│   ├── types (Shared data types and data structures)
│   └── validate (Input validation and checks)
├── Makefile (Build & test commands)
└── go.mod (Dependencies)
```

## ⚙️ Development

A toplevel `Makefile` is provided for convenience:

```console
➜  modbusctl git:(main) $ make
help                           This help
all                            Test and build the application
run                            Run the application
test                           Run tests
build                          Build the application
build-all                      Build the application for all architectures
release-all                    Package the build binaries into tar.gz archives for all architectures
clean                          Clean the build artifacts
```

### Building

```console
make build
make build-all
```

### Running Tests

```console
make test
```


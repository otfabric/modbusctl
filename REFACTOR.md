Here is a concrete refactor proposal for the three deferred files, aimed at improving cohesion without changing behavior.

**Status (done):** The three monolith splits match the “Proposed target layout” below: `internal/config` has `types.go`, `flags.go`, `env.go`, `completion.go`, `diagnostics.go`, `modbus_url.go`, `sunspec.go` (no `config.go`); `internal/mcap` has `codec.go`, `load.go`, `export_*.go`, `decode_profile.go` (no `mcap.go`); `internal/format` has `stitch.go`, `ascii.go`, `frequency.go` plus the other format files (`format.go`, `write.go`, …) — no `helpers.go`. `StitchedBlock` was not present / unused.

**Redeclaration guard:** After the client rename `reportserverid.go` → `report_server_id.go`, both files must never exist together. `make check` runs `check-legacy`, which errors if `cmd/client/reportserverid.go` or any removed monolith path reappears.

Refactor proposal

Goal

Split these oversized files by responsibility, without mixing in rename-only churn and without changing public behavior:
	•	internal/config/config.go *(removed — see `internal/config/*.go`)*
	•	internal/mcap/mcap.go *(removed — see `internal/mcap/*.go`)*
	•	internal/format/helpers.go *(removed — see `stitch.go` / `ascii.go` / `frequency.go`)*

This should be a structure-only refactor first:
	•	no semantic changes
	•	no flag/env/tag changes
	•	no JSON/text output changes
	•	no error message changes unless strictly necessary
	•	keep exported APIs stable where practical

⸻

1. internal/config/config.go

Current problem

This file currently mixes too many concerns:
	•	reflection-based Cobra flag binding
	•	env loading
	•	config struct definitions
	•	diagnostic FC08 lookup tables
	•	shell completion helpers
	•	Modbus URL parsing/validation
	•	SunSpec base parsing
	•	scan algorithm enum/constants

That makes it harder to navigate and increases merge/conflict risk.

Proposed split

internal/config/types.go

Put all config struct definitions here:
	•	DeviceConfig
	•	UnitClientConfig
	•	IdentifyConfig
	•	ReadConfig
	•	RecordConfig
	•	ScanConfig
	•	ConvertConfig
	•	ExtractConfig
	•	StringsConfig
	•	InfoConfig
	•	StaticServerConfig
	•	ReplayServerConfig
	•	DiscoverConfig
	•	FingerprintConfig
	•	DiagnosticConfig
	•	ReportServerIDConfig
	•	DeviceProfileDecodeConfig
	•	SunSpecBaseConfig
	•	SunSpecDetectConfig
	•	SunSpecModelsConfig
	•	SunSpecMapConfig
	•	SunSpecProbeConfig

Also keep:
	•	ScanAlgorithm
	•	ScanAlgo* constants
	•	ScanAlgorithmForExecution

Why: these belong together as the package’s primary domain model.

⸻

internal/config/flags.go

Move:
	•	RegisterFlags

Keep it focused on:
	•	struct tag driven Cobra binding
	•	reflect helpers only

Why: flag registration is its own concern and is currently buried above unrelated config types.

⸻

internal/config/env.go

Move:
	•	LoadFromEnv
	•	MustLoadFromEnv
	•	loadFromEnvStruct

Why: env loading is separate from flag binding and easier to test in isolation.

⸻

internal/config/diagnostics.go

Move:
	•	diagnosticSubFunctions
	•	diagnosticSubFunctionNames
	•	diagnosticSubFunctionMap
	•	init
	•	DiagnosticSubFunctions
	•	ParseDiagnosticSubFunction

Why: this is a compact FC08-specific lookup unit and does not belong in a giant kitchen-sink file.

⸻

internal/config/completion.go

Move:
	•	scanAlgorithmValues
	•	ScanAlgorithms
	•	convertFormatValues
	•	ConvertFormats
	•	ConvertFormatDescriptions
	•	functionCodeValues
	•	FunctionCodes
	•	sunspecRegtypeValues
	•	SunspecRegtypes
	•	ValidScanAlgo
	•	RegisterScanAlgoCompletion
	•	RegisterConvertFormatCompletion
	•	RegisterFunctionCompletion
	•	RegisterDiagnosticSubFunctionCompletion
	•	RegisterRegtypeCompletion

Why: these are all completion/catalog helpers and read naturally together.

⸻

internal/config/modbus_url.go

Move:
	•	ModbusURL
	•	ParseModbusURLHostPort
	•	ValidateModbusAddress
	•	SunSpecModbusURL

Why: URL/address parsing and validation is a distinct responsibility and already forms a coherent cluster.

⸻

internal/config/sunspec.go

Move:
	•	ParseSunSpecBases

Possibly also keep SunSpec-specific constants/helpers here later if they appear.

Why: this is specific parsing logic for SunSpec CLI config.

⸻

Recommended migration order
	1.	Create types.go and move only structs/types first.
	2.	Move RegisterFlags into flags.go.
	3.	Move env loading into env.go.
	4.	Move completion helpers into completion.go.
	5.	Move Modbus URL helpers into modbus_url.go.
	6.	Move diagnostics lookup into diagnostics.go.
	7.	Move ParseSunSpecBases into sunspec.go.
	8.	Delete the old monolith once green.

Expected result

After split, config becomes easy to scan:
	•	“types”
	•	“flags”
	•	“env”
	•	“completion”
	•	“diagnostics”
	•	“url”
	•	“sunspec”

That is a strong cohesion win with very low behavior risk.

⸻

2. internal/mcap/mcap.go

Current problem

This file mixes three different layers:
	•	low-level MCAP binary format I/O
	•	file loading helpers
	•	end-user export/report features

That is too much for one file.

Proposed split

internal/mcap/codec.go

Move the raw format read/write primitives:
	•	mcapMagic
	•	mcapVersion
	•	WriteHeader
	•	AppendRecord
	•	ReadHeader
	•	ReadRecord

Why: these are the binary codec core.

⸻

internal/mcap/load.go

Move:
	•	CountRecords
	•	LoadRecordsFromMCAP

Why: these are file-oriented convenience loaders built on top of the codec.

⸻

internal/mcap/export_csv.go

Move:
	•	ExportCSV

Why: single-purpose export file.

⸻

internal/mcap/export_json.go

Move:
	•	ExportJSON

Why: single-purpose export file.

⸻

internal/mcap/export_blocks.go

Move:
	•	ExportAddressBlocks

Why: distinct export concern.

⸻

internal/mcap/export_info.go

Move:
	•	ExportInfo

Why: reporting/summary export is a separate responsibility.

⸻

internal/mcap/decode_profile.go

Move:
	•	ExportDeviceProfileDecode

Why: this is not a general MCAP codec operation; it is an application-level MCAP + device-profile decode/report flow.

⸻

Notes on boundaries

Keep ExportDeviceProfileDecode in internal/mcap for now even though it is arguably higher-level. The goal here is minimal churn, not package redesign.

Do not rename exported functions yet. Splitting files is enough.

Recommended migration order
	1.	Extract codec.go.
	2.	Extract load.go.
	3.	Move exporters one by one into separate files.
	4.	Run tests after each move.
	5.	Delete original mcap.go.

Expected result

The package becomes much clearer:
	•	codec primitives
	•	load helpers
	•	exporters
	•	profile decode

That will also make later testing easier.

⸻

3. internal/format/helpers.go

Current problem

This file mixes two unrelated feature areas:
	•	record stitching + ASCII extraction
	•	numeric decoder heuristics + frequency detection

Those should not live together under a vague helpers.go.

Also, StitchedBlock appears unused in the shown code and may be removable after verification.

Proposed split

internal/format/stitch.go

Move:
	•	stitchAdjacentRecords
	•	stitchContiguousSameIteration

If StitchedBlock is actually unused repo-wide, remove it. If used elsewhere, place it here.

Why: stitching is its own data-preparation concern.

⸻

internal/format/ascii.go

Move:
	•	ASCIIOrder
	•	ASCIIOrderABCD
	•	ASCIIOrderBADC
	•	ASCIIOrderCDAB
	•	ASCIIOrderDCBA
	•	processASCII
	•	processASCIIForOrder
	•	reorderASCIIBytes
	•	writeASCIICandidate
	•	isPrintableASCII

Why: this is one coherent ASCII extraction pipeline.

⸻

internal/format/frequency.go

Move:
	•	decodeAttempt
	•	decoders
	•	processFrequency
	•	frequencyConfidence
	•	scoreUnscaled
	•	scoreScaled

Why: these are all frequency heuristic internals.

⸻

Small cleanup opportunity during split

There is one likely dead type:
	•	StitchedBlock

I would verify usage first. If unused, delete it as part of the split. That is safe and improves cleanliness.

Recommended migration order
	1.	Move stitching helpers into stitch.go.
	2.	Move ASCII logic into ascii.go.
	3.	Move frequency logic into frequency.go.
	4.	Remove helpers.go.

Expected result

The format package becomes self-explanatory:
	•	stitch.go
	•	ascii.go
	•	frequency.go
	•	existing export/render files

Much easier to navigate.

⸻

Suggested execution plan

Phase 1 — internal/config

Lowest conceptual risk, biggest readability win.

Phase 2 — internal/format

Small file, easy split, good cleanup value.

Phase 3 — internal/mcap

More movement, but still straightforward once the first two are done.

This order reduces risk because:
	•	config is pure structure/workflow glue
	•	format is internal-only helper logic
	•	mcap has the broadest file and most call sites

⸻

Guardrails

For all three refactors:
	•	keep package name unchanged
	•	keep exported symbol names unchanged unless there is a compelling reason
	•	keep comments with the moved functions
	•	avoid opportunistic rewrites
	•	run tests after each extracted file
	•	run go test ./... after each phase, not just at the end

⸻

Proposed target layout

internal/config
	•	types.go
	•	flags.go
	•	env.go
	•	completion.go
	•	diagnostics.go
	•	modbus_url.go
	•	sunspec.go

internal/mcap
	•	codec.go
	•	load.go
	•	export_csv.go
	•	export_json.go
	•	export_blocks.go
	•	export_info.go
	•	decode_profile.go

internal/format
	•	stitch.go
	•	ascii.go
	•	frequency.go

⸻

Success criteria

The refactor is done when:
	•	the three monolith files are gone
	•	behavior is unchanged
	•	public APIs are unchanged
	•	tests remain green
	•	file names describe a single responsibility each

If you want, I can next turn this into a concrete step-by-step implementation checklist with exact move groups and likely import fallout per step.
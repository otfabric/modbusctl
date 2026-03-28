package config

import "strings"

type DeviceConfig struct {
	URL  string `env:"MODBUSCTL_URL" desc:"Modbus URL (e.g. tcp://192.168.1.10:502); mutually exclusive with --ip/--port" flag:"url"`
	IP   string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	Unit uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID of the Modbus device" flag:"unit"`
}

// UnitClientConfig is used by commands that support a single unit ID or "all" (1–255).
type UnitClientConfig struct {
	URL      string `env:"MODBUSCTL_URL" desc:"Modbus URL (e.g. tcp://192.168.1.10:502); mutually exclusive with --ip/--port" flag:"url"`
	IP       string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port     uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	UnitID   string `env:"MODBUSCTL_UNIT" desc:"Unit ID: single (1), range (1-10), list (1,5,25), mixed (1-10,255), or 'all' (1-255)" flag:"unit"`
	Timeout  uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in ms per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Parallel uint16 `env:"MODBUSCTL_PARALLEL" desc:"When --unit all, number of concurrent probes (1-64, ignored otherwise)" flag:"parallel"`
}

type IdentifyConfig struct {
	UnitClientConfig
	Debug        bool   // set from global --debug (root persistent flag)
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Basic        bool   `env:"MODBUSCTL_IDENTIFY_BASIC" desc:"Request Basic category only (VendorName, ProductCode, MajorMinorRevision)" flag:"basic"`
	Regular      bool   `env:"MODBUSCTL_IDENTIFY_REGULAR" desc:"Request Regular category only (Basic + VendorUrl, ProductName, ModelName, UserApplicationName)" flag:"regular"`
	Extended     bool   `env:"MODBUSCTL_IDENTIFY_EXTENDED" desc:"Request Extended category only (Regular + vendor-specific objects)" flag:"extended"`
	ServerID     bool   `env:"MODBUSCTL_IDENTIFY_SERVERID" desc:"Also query FC17 Report Server ID for additional device information" flag:"server-id"`
}

type ReadConfig struct {
	DeviceConfig
	Timeout       uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Function      uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	StartAddress  uint16 `env:"MODBUSCTL_ADDRESS" desc:"Starting register address" flag:"start"`
	RegisterCount uint16 `env:"MODBUSCTL_COUNT" desc:"Number of registers to read" flag:"count"`
	Ascii         bool   `env:"MODBUSCTL_ASCII" desc:"Attempt ASCII decoding for output" flag:"ascii"`
	SwapBytes     bool   `env:"MODBUSCTL_BYTESWAP" desc:"Enable byte swapping for registers" flag:"byteswap"`
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output MCAP file or directory" flag:"output"`
	OutputFormat  string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug         bool   // set from global --debug (root persistent flag)
}

type RecordConfig struct {
	DeviceConfig
	Timeout      uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Function     uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	Interval     uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between reads" flag:"interval"`
	Duration     uint32 `env:"MODBUSCTL_DURATION" desc:"Total duration to record in milliseconds" flag:"duration"`
	BlocksFile   string `env:"MODBUSCTL_BLOCKS_FILE" desc:"JSON file of address blocks [{start_address, register_count}, ...]" flag:"blocks-file"`
	OutputFile   string `env:"MODBUSCTL_OUTPUT" desc:"Output MCAP file or directory" flag:"output"`
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug        bool   // set from global --debug (root persistent flag)
}

// ScanAlgorithm selects the scan strategy: safe (conservative linear), smart (interval splitting), or deep (smart + boundary refinement).
type ScanAlgorithm string

const (
	ScanAlgoSafe     ScanAlgorithm = "safe"
	ScanAlgoSmart    ScanAlgorithm = "smart"
	ScanAlgoDeep     ScanAlgorithm = "deep"
	ScanAlgoStepped  ScanAlgorithm = "stepped"
	ScanAlgoLinear   ScanAlgorithm = "linear"
	ScanAlgoBoundary ScanAlgorithm = "boundary"
	ScanAlgoSunspec  ScanAlgorithm = "sunspec"
)

// ScanAlgorithmForExecution returns NormalizedAlgo when set (after validation), otherwise
// lower-trims cfg.Algo with empty treated as safe. Used by the scan executor and tests that bypass validation.
func ScanAlgorithmForExecution(cfg *ScanConfig) ScanAlgorithm {
	if cfg == nil {
		return ScanAlgoSafe
	}
	if cfg.NormalizedAlgo != "" {
		return cfg.NormalizedAlgo
	}
	a := strings.ToLower(strings.TrimSpace(cfg.Algo))
	if a == "" {
		return ScanAlgoSafe
	}
	return ScanAlgorithm(a)
}

type ScanConfig struct {
	DeviceConfig
	Timeout                 uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Delay                   uint16 `env:"MODBUSCTL_DELAY" desc:"Delay in milliseconds between requests" flag:"delay"`
	Function                uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	StartAddress            uint16 `env:"MODBUSCTL_START" desc:"Start register address" flag:"start"`
	EndAddress              uint16 `env:"MODBUSCTL_END" desc:"End register address" flag:"end"`
	OutputFile              string `env:"MODBUSCTL_OUTPUT" desc:"Output file" flag:"output"`
	Algo                    string `env:"MODBUSCTL_ALGO" desc:"Scan algorithm: safe, smart, deep, stepped, linear, boundary, or sunspec" flag:"algo"`
	Step                    uint16 `env:"MODBUSCTL_STEP" desc:"Stepped algo: step size (e.g. 100, 1000, 2000)" flag:"step"`
	StepHalfOffset          bool   `env:"MODBUSCTL_STEP_HALF_OFFSET" desc:"Stepped algo: also probe at step/2 offset" flag:"step-half-offset"`
	SeedStart               uint16 `env:"MODBUSCTL_SEED_START" desc:"Boundary algo: seed start address (known good read)" flag:"seed-start"`
	SeedCount               uint16 `env:"MODBUSCTL_SEED_COUNT" desc:"Boundary algo: seed register count (1-125)" flag:"seed-count"`
	RetryOnTimeoutTransport uint8  `env:"MODBUSCTL_RETRY_TIMEOUT" desc:"Retry once on timeout or transport error (0=no, 1=yes)" flag:"retry-timeout"`
	SunSpecBase             uint16 `env:"MODBUSCTL_SUNSPEC_BASE" desc:"Sunspec algo: known base address (skip detection)" flag:"sunspec-base"`
	SunSpecBases            string `env:"MODBUSCTL_SUNSPEC_BASES" desc:"Sunspec algo: comma-separated candidate base addresses" flag:"sunspec-bases"`
	SunSpecMaxModels        int    `env:"MODBUSCTL_SUNSPEC_MAX_MODELS" desc:"Sunspec algo: max model headers to read (0=256)" flag:"sunspec-max-models"`
	SunSpecMaxSpan          uint16 `env:"MODBUSCTL_SUNSPEC_MAX_SPAN" desc:"Sunspec algo: max address span from base (0=no limit)" flag:"sunspec-max-span"`
	OutputFormat            string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table (final summary on stdout; progress on stderr)" flag:"format"`
	Debug                   bool   // set from global --debug (root persistent flag)
	// NormalizedAlgo is set by [validate.CheckScanConfig] to the canonical lowercase algorithm (including ""→safe).
	// Not loaded from env or flags.
	NormalizedAlgo ScanAlgorithm `json:"-"`
}

type ConvertConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	FormatType string `env:"MODBUSCTL_FORMAT" desc:"Output format type (e.g., CSV, JSON)" flag:"format"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type ExtractConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StringsConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type InfoConfig struct {
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StaticServerConfig struct {
	Port      uint16 `env:"MODBUSCTL_PORT" desc:"Port for the static server" flag:"port"`
	Unit      uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID for the Modbus device" flag:"unit"`
	Function  uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (1–4); 0=use MCAP header FC (default). If set, must match the file." flag:"function"`
	InputFile string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file to serve" flag:"input"`
	Debug     bool   // set from global --debug (root persistent flag)
}

type ReplayServerConfig struct {
	Port      uint16 `env:"MODBUSCTL_PORT" desc:"Port for the replay server" flag:"port"`
	Unit      uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID for the Modbus device" flag:"unit"`
	Function  uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (1–4); 0=use MCAP header FC (default). If set, must match the file." flag:"function"`
	Loops     uint16 `env:"MODBUSCTL_LOOPS" desc:"Number of times to loop the replay" flag:"loops"`
	Interval  uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between iterations (0 for original timing)" flag:"interval"`
	InputFile string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file to replay" flag:"input"`
	Debug     bool   // set from global --debug (root persistent flag)
}

type DiscoverConfig struct {
	Subnets          []string `env:"MODBUSCTL_SUBNETS" desc:"List of subnets to scan for Modbus devices (e.g., 192.168.1.0/24,192.168.2.0/24)" flag:"subnets"`
	Port             uint16   `env:"MODBUSCTL_PORT" desc:"Port to use for Modbus TCP discovery" flag:"port"`
	Parallel         uint8    `env:"MODBUSCTL_PARALLEL" desc:"Parallel discovery workers (1-64)" flag:"parallel"`
	ResolveMAC       bool     `env:"MODBUSCTL_RESOLVE_MAC" desc:"Resolve MAC addresses of discovered devices" flag:"resolve-mac"`
	NetworkInterface string   `env:"MODBUSCTL_INTERFACE" desc:"Network interface to use for discovery" flag:"interface"`
	OutputFile       string   `env:"MODBUSCTL_OUTPUT" desc:"Output file for discovered devices" flag:"output"`
	OutputFormat     string   `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Stdout format: text, json, or table" flag:"format"`
	ForceLargeScan   bool     `env:"MODBUSCTL_DISCOVER_FORCE_LARGE" desc:"Allow discovery when unique host count exceeds the safety cap (default 65536)" flag:"force-large-scan"`
	Debug            bool     // set from global --debug (root persistent flag)
}

// FingerprintConfig is used by the fingerprint command (probe supported read FCs per unit via HasUnitReadFunction).
type FingerprintConfig struct {
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL (e.g. tcp://192.168.1.10:502); mutually exclusive with --ip/--port" flag:"url"`
	IP           string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port         uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	UnitID       string `env:"MODBUSCTL_UNIT" desc:"Unit ID: single, range (1-10), list (1,5,25), mixed (1-10,255), or 'all'" flag:"unit"`
	Timeout      uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in ms per probe and TCP dial cap (0 = 10000)" flag:"timeout"`
	Interval     uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between probes" flag:"interval"`
	Debug        bool   // set from global --debug (root persistent flag)
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// DiagnosticConfig is used by the diagnostic command (FC08 Diagnostics).
type DiagnosticConfig struct {
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL (e.g. tcp://192.168.1.10:502); mutually exclusive with --ip/--port" flag:"url"`
	IP           string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port         uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	UnitID       uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID of the Modbus device" flag:"unit"`
	Timeout      uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in ms for the request and TCP dial cap (0 = 10000)" flag:"timeout"`
	SubFunction  string `env:"MODBUSCTL_SUB_FUNCTION" desc:"FC08 sub-function name (e.g. returnquerydata, clearcountersanddiagnosticreg)" flag:"sub-function"`
	Data         string `env:"MODBUSCTL_DATA" desc:"Hex-encoded request data (e.g. 'A537'); defaults to 0000" flag:"data"`
	Debug        bool   // set from global --debug (root persistent flag)
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// ReportServerIDConfig is used by the reportserverid command (FC17 Report Server ID).
type ReportServerIDConfig struct {
	UnitClientConfig
	Debug        bool   // set from global --debug (root persistent flag)
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

type DeviceProfileDecodeConfig struct {
	InputFile     string `env:"MODBUSCTL_INPUT" desc:"Input MCAP file with device profile" flag:"input"`
	DeviceProfile string `env:"MODBUSCTL_PROFILE" desc:"Device profile to decode" flag:"profile"`
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output file for decoded profile" flag:"output"`
}

// SunSpecBaseConfig is shared by all client sunspec commands (URL or IP/Port, unit, regtype, output).
type SunSpecBaseConfig struct {
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL (e.g. tcp://192.168.1.10:502); overrides --ip/--port when set" flag:"url"`
	IP           string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port         uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	Unit         uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID (0-255, full Modbus range)" flag:"unit"`
	Timeout      uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Modbus client/request timeout in ms (0=10000); SunSpec detect/model-header work uses a derived operation budget (min 5s, max 2m, 2× this timeout)" flag:"timeout"`
	Regtype      string `env:"MODBUSCTL_REGTYPE" desc:"Register type: holding (FC03) or input (FC04)" flag:"regtype"`
	Verbose      bool   `env:"MODBUSCTL_VERBOSE" desc:"Show probe attempts or extra detail" flag:"verbose"`
	Debug        bool   // set from global --debug (root persistent flag)
	OutputFormat string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
}

// SunSpecDetectConfig is used by client sunspec detect.
type SunSpecDetectConfig struct {
	SunSpecBaseConfig
	Bases string `env:"MODBUSCTL_SUNSPEC_BASES" desc:"Comma-separated base addresses to probe (e.g. 0,40000,50000)" flag:"bases"`
}

// SunSpecModelsConfig is used by client sunspec models.
type SunSpecModelsConfig struct {
	SunSpecBaseConfig
	Base           uint16 `env:"MODBUSCTL_SUNSPEC_BASE" desc:"Known SunSpec base address; skip detection when set" flag:"base"`
	MaxModels      int    `env:"MODBUSCTL_SUNSPEC_MAX_MODELS" desc:"Maximum model headers to read (0 = 256)" flag:"max-models"`
	MaxAddressSpan uint16 `env:"MODBUSCTL_SUNSPEC_MAX_SPAN" desc:"Maximum address span from base (0 = no limit)" flag:"max-address-span"`
}

// SunSpecMapConfig is used by client sunspec map.
type SunSpecMapConfig struct {
	SunSpecBaseConfig
	Base           uint16 `env:"MODBUSCTL_SUNSPEC_BASE" desc:"Known SunSpec base address; skip detection when set" flag:"base"`
	ShowHeaderRegs bool   `env:"MODBUSCTL_SUNSPEC_SHOW_HEADER" desc:"Show header register ranges" flag:"show-header-regs"`
	ShowNext       bool   `env:"MODBUSCTL_SUNSPEC_SHOW_NEXT" desc:"Show next-address column" flag:"show-next"`
	Compact        bool   `env:"MODBUSCTL_SUNSPEC_COMPACT" desc:"Compact one-line per model" flag:"compact"`
}

// SunSpecProbeConfig is used by client sunspec probe.
type SunSpecProbeConfig struct {
	SunSpecBaseConfig
}

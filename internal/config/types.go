package config

import "strings"

// SerialConfig holds the serial-line settings used with rtu:// and ascii:// URLs.
// Zero values select the defaults documented on each flag; the settings are ignored
// for network URLs.
type SerialConfig struct {
	Baud     uint   `env:"MODBUSCTL_BAUD" desc:"Serial baud rate for rtu:// and ascii:// URLs (0 = 19200)" flag:"baud"`
	DataBits uint   `env:"MODBUSCTL_DATA_BITS" desc:"Serial data bits, 5-8 (0 = 8 for rtu://, 7 for ascii://)" flag:"data-bits"`
	Parity   string `env:"MODBUSCTL_PARITY" desc:"Serial parity: none, even, or odd (default none)" flag:"parity"`
	StopBits uint   `env:"MODBUSCTL_STOP_BITS" desc:"Serial stop bits, 1 or 2 (0 = 2 without parity, 1 with parity)" flag:"stop-bits"`
}

type DeviceConfig struct {
	SerialConfig
	TLSConfig
	URL  string `env:"MODBUSCTL_URL" desc:"Modbus URL: tcp://, tcp+tls://, udp://, rtu://, ascii://, rtuovertcp://, rtuoverudp:// or asciiovertcp:// (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port" flag:"url"`
	IP   string `env:"MODBUSCTL_IP" desc:"Modbus TCP device IP (used when --url is not set)" flag:"ip"`
	Port uint16 `env:"MODBUSCTL_PORT" desc:"Modbus TCP port (used when --url is not set)" flag:"port"`
	Unit uint8  `env:"MODBUSCTL_UNIT" desc:"Unit ID of the Modbus device" flag:"unit"`
}

// UnitClientConfig is used by commands that support a single unit ID or "all" (1–255).
type UnitClientConfig struct {
	SerialConfig
	TLSConfig
	URL      string `env:"MODBUSCTL_URL" desc:"Modbus URL: tcp://, tcp+tls://, udp://, rtu://, ascii://, rtuovertcp://, rtuoverudp:// or asciiovertcp:// (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port" flag:"url"`
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
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output capture file (.mbcap) or directory" flag:"output"`
	OutputFormat  string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug         bool   // set from global --debug (root persistent flag)
}

// DecodeConfig drives `client decode`: read holding/input registers and decode
// them into typed values using a go-modbus codec (see `client codecs`).
type DecodeConfig struct {
	DeviceConfig
	Timeout       uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Function      uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (3=holding, 4=input)" flag:"function"`
	StartAddress  uint16 `env:"MODBUSCTL_ADDRESS" desc:"Starting register address" flag:"start"`
	Codec         string `env:"MODBUSCTL_CODEC" desc:"Codec ID (e.g. float32/layout:4321, ascii/registers:8, ip_addr); see 'client codecs'" flag:"codec"`
	RegisterCount uint16 `env:"MODBUSCTL_COUNT" desc:"Registers to read (0 = one codec value; otherwise a multiple of the codec register width)" flag:"count"`
	OutputFormat  string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug         bool   // set from global --debug (root persistent flag)
}

// WriteConfig drives `client write`: write coils (FC5/FC15) or holding registers (FC6/FC16).
type WriteConfig struct {
	DeviceConfig
	Timeout      uint16   `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and dial cap (0 = 10000)" flag:"timeout"`
	Function     uint8    `env:"MODBUSCTL_FUNCTION" desc:"Function code: 5 (single coil), 6 (single register), 15 (multiple coils), 16 (multiple registers); 0 = 6 for one register value, otherwise 16" flag:"function"`
	StartAddress uint16   `env:"MODBUSCTL_ADDRESS" desc:"Address of the first coil or register to write" flag:"start"`
	Values       []string `env:"MODBUSCTL_VALUES" desc:"Comma-separated values to write: registers 0-65535, 0x hex or -32768..-1; coils 1/0, true/false or on/off; with --codec one typed value per entry" flag:"values"`
	Codec        string   `env:"MODBUSCTL_CODEC" desc:"Encode each value with this codec before writing (e.g. float32/layout:4321); see 'client codecs'" flag:"codec"`
	OutputFormat string   `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json, or table" flag:"format"`
	Debug        bool     // set from global --debug (root persistent flag)
}

// PollConfig drives `client poll`: read the same block repeatedly and print every sample.
type PollConfig struct {
	DeviceConfig
	Timeout       uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and dial cap (0 = 10000)" flag:"timeout"`
	Function      uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code (1=coils, 2=discrete inputs, 3=holding, 4=input)" flag:"function"`
	StartAddress  uint16 `env:"MODBUSCTL_ADDRESS" desc:"Address of the first coil or register to poll" flag:"start"`
	RegisterCount uint16 `env:"MODBUSCTL_COUNT" desc:"Coils or registers per poll (with --codec: 0 = one value, otherwise a multiple of the codec register width)" flag:"count"`
	Codec         string `env:"MODBUSCTL_CODEC" desc:"Decode registers with this codec (functions 3 and 4 only); see 'client codecs'" flag:"codec"`
	Interval      uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between the start of consecutive polls" flag:"interval"`
	Polls         uint32 `env:"MODBUSCTL_POLLS" desc:"Number of polls before exiting (0 = until interrupted)" flag:"polls"`
	OutputFormat  string `env:"MODBUSCTL_OUTPUT_FORMAT" desc:"Output format: text, json (one object per line), or table" flag:"format"`
	Debug         bool   // set from global --debug (root persistent flag)
}

type RecordConfig struct {
	DeviceConfig
	Timeout      uint16 `env:"MODBUSCTL_TIMEOUT" desc:"Timeout in milliseconds per Modbus request and TCP dial cap (0 = 10000)" flag:"timeout"`
	Function     uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code for address blocks without one: 1=coils, 2=discrete inputs, 3=holding, 4=input" flag:"function"`
	Interval     uint32 `env:"MODBUSCTL_INTERVAL" desc:"Interval in milliseconds between reads" flag:"interval"`
	Duration     uint32 `env:"MODBUSCTL_DURATION" desc:"Total duration to record in milliseconds" flag:"duration"`
	BlocksFile   string `env:"MODBUSCTL_BLOCKS_FILE" desc:"JSON recording plan: address blocks [{start_address, register_count}, ...] read with --function, and/or requests of any read function [{function, ...}] (see README)" flag:"blocks-file"`
	OutputFile   string `env:"MODBUSCTL_OUTPUT" desc:"Output capture file (.mbcap) or directory" flag:"output"`
	Exceptions   bool   `env:"MODBUSCTL_RECORD_EXCEPTIONS" desc:"Also record exception responses in the capture" flag:"record-exceptions"`
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
	OutputFile              string `env:"MODBUSCTL_OUTPUT" desc:"Output capture file (.mbcap) or directory" flag:"output"`
	Exceptions              bool   `env:"MODBUSCTL_RECORD_EXCEPTIONS" desc:"Also record exception responses in the capture" flag:"record-exceptions"`
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

// CaptureSelection picks the FC01-04 samples of one target and one function from a
// capture, for the commands that work on register blocks.
type CaptureSelection struct {
	Target   uint32 `env:"MODBUSCTL_TARGET" desc:"Capture target to use (0 = the only one)" flag:"target"`
	Function uint8  `env:"MODBUSCTL_FUNCTION" desc:"Function code to use, 1-4 (0 = the only one)" flag:"function"`
}

type ExtractConfig struct {
	CaptureSelection
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Capture file (.mbcap, .mbcap.jsonl or .mbcap.csv)" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StringsConfig struct {
	CaptureSelection
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Capture file (.mbcap, .mbcap.jsonl or .mbcap.csv)" flag:"input"`
	OutputFile string `env:"MODBUSCTL_OUTPUT" desc:"Output file or directory" flag:"output"`
}

type StaticServerConfig struct {
	ServerTLSConfig
	Listen     string `env:"MODBUSCTL_LISTEN" desc:"Address to listen on, e.g. 127.0.0.1 (default: every interface)" flag:"listen"`
	Port       uint16 `env:"MODBUSCTL_PORT" desc:"Port for the static server" flag:"port"`
	Unit       uint8  `env:"MODBUSCTL_UNIT" desc:"Serve the target as this unit id (0 = the unit id recorded for each target)" flag:"unit"`
	Target     uint32 `env:"MODBUSCTL_TARGET" desc:"Capture target to serve (0 = every target, each on its recorded unit id)" flag:"target"`
	At         string `env:"MODBUSCTL_AT" desc:"Serve the values at this time since the capture start, e.g. 90s (default: the capture end, i.e. the last values)" flag:"at"`
	Missing    string `env:"MODBUSCTL_MISSING" desc:"Answer for data the capture does not hold: exception (default), zero, or silence" flag:"missing"`
	MaxClients uint16 `env:"MODBUSCTL_MAX_CLIENTS" desc:"Maximum concurrent client connections (0 = 10); further connections are closed" flag:"max-clients"`
	InputFile  string `env:"MODBUSCTL_INPUT" desc:"Capture file to serve (.mbcap, .mbcap.jsonl or .mbcap.csv)" flag:"input"`
	Debug      bool   // set from global --debug (root persistent flag)
}

type ReplayServerConfig struct {
	ServerTLSConfig
	Listen      string `env:"MODBUSCTL_LISTEN" desc:"Address to listen on, e.g. 127.0.0.1 (default: every interface)" flag:"listen"`
	Port        uint16 `env:"MODBUSCTL_PORT" desc:"Port for the replay server" flag:"port"`
	Unit        uint8  `env:"MODBUSCTL_UNIT" desc:"Serve the target as this unit id (0 = the unit id recorded for each target)" flag:"unit"`
	Target      uint32 `env:"MODBUSCTL_TARGET" desc:"Capture target to serve (0 = every target, each on its recorded unit id)" flag:"target"`
	Speed       string `env:"MODBUSCTL_SPEED" desc:"Replay speed: 1 = original (default), 0.1 = ten times slower, 60 = a minute per second" flag:"speed"`
	Loops       uint16 `env:"MODBUSCTL_LOOPS" desc:"Times to play the capture (0 = forever, the default); afterwards the last values stay" flag:"loops"`
	From        string `env:"MODBUSCTL_FROM" desc:"Start the replay at this time since the capture start, e.g. 10m (default 0)" flag:"from"`
	Missing     string `env:"MODBUSCTL_MISSING" desc:"Answer for data the capture does not hold: exception (default), zero, or silence" flag:"missing"`
	BeforeFirst string `env:"MODBUSCTL_BEFORE_FIRST" desc:"Before an address is first sampled: hold (serve its first sample, default) or absent" flag:"before-first"`
	MaxClients  uint16 `env:"MODBUSCTL_MAX_CLIENTS" desc:"Maximum concurrent client connections (0 = 10); further connections are closed" flag:"max-clients"`
	InputFile   string `env:"MODBUSCTL_INPUT" desc:"Capture file to replay (.mbcap, .mbcap.jsonl or .mbcap.csv)" flag:"input"`
	Debug       bool   // set from global --debug (root persistent flag)
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

// FingerprintConfig is used by the fingerprint command (probe supported read FCs per unit via go-modbus SupportsFunction).
type FingerprintConfig struct {
	SerialConfig
	TLSConfig
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL: tcp://, tcp+tls://, udp://, rtu://, ascii://, rtuovertcp://, rtuoverudp:// or asciiovertcp:// (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port" flag:"url"`
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
	SerialConfig
	TLSConfig
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL: tcp://, tcp+tls://, udp://, rtu://, ascii://, rtuovertcp://, rtuoverudp:// or asciiovertcp:// (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port" flag:"url"`
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
	CaptureSelection
	InputFile     string `env:"MODBUSCTL_INPUT" desc:"Capture file (.mbcap, .mbcap.jsonl or .mbcap.csv)" flag:"input"`
	DeviceProfile string `env:"MODBUSCTL_PROFILE" desc:"Device profile to decode" flag:"profile"`
	OutputFile    string `env:"MODBUSCTL_OUTPUT" desc:"Output file for decoded profile" flag:"output"`
}

// SunSpecBaseConfig is shared by all client sunspec commands (URL or IP/Port, unit, regtype, output).
type SunSpecBaseConfig struct {
	SerialConfig
	TLSConfig
	URL          string `env:"MODBUSCTL_URL" desc:"Modbus URL: tcp://, tcp+tls://, udp://, rtu://, ascii://, rtuovertcp://, rtuoverudp:// or asciiovertcp:// (e.g. tcp://192.168.1.10:502, rtu:///dev/ttyUSB0); mutually exclusive with --ip/--port" flag:"url"`
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
